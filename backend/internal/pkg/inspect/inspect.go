package inspect

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"sigma-tst/backend/internal/pkg/types"
)

type Snapshot struct {
	URL        string      `json:"url"`
	Title      string      `json:"title"`
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Index              int      `json:"index"`
	Tag                string   `json:"tag"`
	Role               string   `json:"role"`
	Type               string   `json:"type"`
	Text               string   `json:"text"`
	Label              string   `json:"label"`
	Name               string   `json:"name"`
	Placeholder        string   `json:"placeholder"`
	Href               string   `json:"href"`
	Disabled           bool     `json:"disabled"`
	NearbyText         string   `json:"nearbyText"`
	SuggestedSelectors []string `json:"suggestedSelectors"`
}

func URLFromIntent(intent string) string {
	re := regexp.MustCompile(`https?://[^\s,")]+`)
	match := re.FindString(intent)
	return strings.TrimRight(match, "/.")
}

func Page(projectRoot, intent string) (*Snapshot, error) {
	targetURL := URLFromIntent(intent)
	if targetURL == "" {
		return nil, nil
	}

	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("sigma-page-inspect-%d.json", time.Now().UnixMilli()))
	cmd := exec.Command("node", filepath.Join(projectRoot, "runner", "inspect.js"), "--url", targetURL, "--out", outPath)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("page inspection failed: %v: %s", err, string(output))
	}
	defer os.Remove(outPath)

	raw, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}

	var snapshot Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func GroundSpec(spec *types.TestSpec, snapshot *Snapshot) {
	if spec == nil || snapshot == nil {
		return
	}
	for i := range spec.Steps {
		step := &spec.Steps[i]
		if step.Action != "click" || strings.TrimSpace(step.Selector) == "" {
			continue
		}
		best := bestCandidateForStep(*step, snapshot.Candidates)
		if best == nil || len(best.SuggestedSelectors) == 0 {
			continue
		}
		step.Selector = best.SuggestedSelectors[0]
	}
}

func ProgressiveGroundSpec(projectRoot string, spec *types.TestSpec) error {
	if spec == nil {
		return nil
	}

	inPath := filepath.Join(os.TempDir(), fmt.Sprintf("sigma-ground-in-%d.json", time.Now().UnixMilli()))
	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("sigma-ground-out-%d.json", time.Now().UnixMilli()))
	defer os.Remove(inPath)
	defer os.Remove(outPath)

	raw, _ := json.Marshal(spec)
	if err := os.WriteFile(inPath, raw, 0o644); err != nil {
		return err
	}

	cmd := exec.Command("node", filepath.Join(projectRoot, "runner", "ground.js"), "--spec", inPath, "--out", outPath)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("progressive grounding failed: %v: %s", err, string(output))
	}

	groundedRaw, err := os.ReadFile(outPath)
	if err != nil {
		return err
	}

	var grounded types.TestSpec
	if err := json.Unmarshal(groundedRaw, &grounded); err != nil {
		return err
	}
	*spec = grounded
	return nil
}

func bestCandidateForStep(step types.Step, candidates []Candidate) *Candidate {
	needle := strings.ToLower(strings.Join([]string{step.Target, step.Description, step.Selector}, " "))
	words := keywordSet(needle)
	var best *Candidate
	bestScore := 0

	for i := range candidates {
		candidate := &candidates[i]
		if candidate.Disabled || len(candidate.SuggestedSelectors) == 0 {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			candidate.Text,
			candidate.Label,
			candidate.Name,
			candidate.Placeholder,
			candidate.Href,
			candidate.NearbyText,
		}, " "))
		score := 0
		for word := range words {
			if strings.Contains(haystack, word) {
				score += 20
			}
		}
		if candidate.Text != "" && strings.Contains(needle, strings.ToLower(candidate.Text)) {
			score += 25
		}
		if candidate.NearbyText != "" {
			for word := range words {
				if strings.Contains(strings.ToLower(candidate.NearbyText), word) {
					score += 10
				}
			}
		}
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}

	if bestScore < 25 {
		return nil
	}
	return best
}

func keywordSet(value string) map[string]struct{} {
	stopWords := map[string]struct{}{
		"click": {}, "open": {}, "the": {}, "and": {}, "to": {}, "on": {}, "complete": {}, "homepage": {},
		"button": {}, "link": {}, "role": {}, "name": {}, "has": {}, "text": {},
	}
	words := regexp.MustCompile(`[a-z0-9]+`).FindAllString(strings.ToLower(value), -1)
	result := make(map[string]struct{})
	for _, word := range words {
		if len(word) < 3 {
			continue
		}
		if _, ok := stopWords[word]; ok {
			continue
		}
		result[word] = struct{}{}
	}
	return result
}
