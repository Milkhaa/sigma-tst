package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"sigma-tst/backend/internal/pkg/types"
)

func Execute(spec types.TestSpec, projectRoot string) (types.RunResult, error) {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	started := time.Now().UTC()

	artifactDir := filepath.Join(projectRoot, "artifacts", runID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return types.RunResult{}, err
	}
	specPath := filepath.Join(artifactDir, "spec.json")
	outPath := filepath.Join(artifactDir, "result.json")

	specBytes, _ := json.Marshal(spec)
	if err := os.WriteFile(specPath, specBytes, 0o644); err != nil {
		return types.RunResult{}, err
	}

	args := []string{
		filepath.Join(projectRoot, "runner", "index.js"),
		"--spec", specPath,
		"--out", outPath,
		"--artifactDir", artifactDir,
		"--runId", runID,
	}
	cmd := exec.Command("node", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return types.RunResult{}, fmt.Errorf("runner failed: %v: %s", err, string(output))
	}

	var result types.RunResult
	resultBytes, err := os.ReadFile(outPath)
	if err != nil {
		return types.RunResult{}, err
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return types.RunResult{}, err
	}
	result.RunID = runID
	result.StartedAt = started.Format(time.RFC3339)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return result, nil
}
