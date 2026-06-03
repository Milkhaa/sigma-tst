package promotions

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"sigma-tst/backend/internal/pkg/respond"
	"sigma-tst/backend/internal/pkg/store"
	"sigma-tst/backend/internal/pkg/types"
)

type Impl struct {
	ProjectRoot string
}

func (impl *Impl) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respond.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		impl.handle(w, r)
	}
}

func (impl *Impl) handle(w http.ResponseWriter, r *http.Request) {
	candidateID, action, ok := parsePath(r.URL.Path)
	if !ok {
		respond.JSON(w, http.StatusNotFound, map[string]string{"error": "expected /v1/promotions/:candidateId/approve or /reject"})
		return
	}

	var req types.PromotionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(req.RunID) == "" {
		respond.JSON(w, http.StatusBadRequest, map[string]string{"error": "runId is required"})
		return
	}

	result, err := store.LoadRunResult(impl.ProjectRoot, req.RunID)
	if err != nil {
		respond.JSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	candidate, err := findCandidate(&result, candidateID)
	if err != nil {
		respond.JSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	switch action {
	case "approve":
		impl.approve(w, req, &result, candidate)
	case "reject":
		impl.reject(w, req, &result, candidate)
	default:
		respond.JSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
	}
}

func (impl *Impl) approve(w http.ResponseWriter, req types.PromotionRequest, result *types.RunResult, candidate *types.PromotionCandidate) {
	updatedSpec, promotedPath, err := applyPromotion(impl.ProjectRoot, req.RunID, candidate)
	if err != nil {
		log.Error().Err(err).Str("candidate_id", candidate.ID).Msg("promotions.approve_error")
		respond.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	candidate.Status = "approved"
	candidate.ReviewReason = req.Reason
	candidate.PromotedSpecPath = promotedPath
	syncCandidate(result, candidate)
	*result, _ = store.SaveRunResult(impl.ProjectRoot, *result)
	respond.JSON(w, http.StatusOK, types.PromotionResponse{Promotion: *candidate, UpdatedSpec: updatedSpec, RunResult: result})
}

func (impl *Impl) reject(w http.ResponseWriter, req types.PromotionRequest, result *types.RunResult, candidate *types.PromotionCandidate) {
	candidate.Status = "rejected"
	candidate.ReviewReason = req.Reason
	syncCandidate(result, candidate)
	*result, _ = store.SaveRunResult(impl.ProjectRoot, *result)
	respond.JSON(w, http.StatusOK, types.PromotionResponse{Promotion: *candidate, RunResult: result})
}

func applyPromotion(projectRoot, runID string, candidate *types.PromotionCandidate) (*types.TestSpec, string, error) {
	specPath := filepath.Join(projectRoot, "artifacts", runID, "spec.json")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, "", fmt.Errorf("original spec not found for %s", runID)
	}

	var spec types.TestSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, "", err
	}

	updated := false
	for i := range spec.Steps {
		if spec.Steps[i].ID != candidate.StepID {
			continue
		}
		spec.Steps[i].Action = candidate.ProposedAction
		spec.Steps[i].Selector = candidate.ProposedSelector
		spec.Steps[i].Ungrounded = false
		spec.Steps[i].GroundingNote = ""
		updated = true
		break
	}
	if !updated {
		return nil, "", errors.New("candidate step not found in original spec")
	}

	promotedPath := filepath.Join(projectRoot, "artifacts", runID, "promoted-spec.json")
	if err := respond.JSONFile(promotedPath, spec); err != nil {
		return nil, "", err
	}

	abs, _ := filepath.Abs(promotedPath)
	return &spec, abs, nil
}

func findCandidate(result *types.RunResult, candidateID string) (*types.PromotionCandidate, error) {
	for i := range result.PromotionCandidates {
		if result.PromotionCandidates[i].ID == candidateID {
			return &result.PromotionCandidates[i], nil
		}
	}
	return nil, fmt.Errorf("promotion candidate not found: %s", candidateID)
}

func syncCandidate(result *types.RunResult, candidate *types.PromotionCandidate) {
	for i := range result.Steps {
		if result.Steps[i].PromotionCandidate == nil {
			continue
		}
		if result.Steps[i].PromotionCandidate.ID == candidate.ID {
			result.Steps[i].PromotionCandidate = candidate
		}
	}
}

func parsePath(path string) (candidateID, action string, ok bool) {
	rest := strings.Trim(strings.TrimPrefix(path, "/v1/promotions/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
