package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"sigma-tst/backend/internal/pkg/report"
	"sigma-tst/backend/internal/pkg/respond"
	"sigma-tst/backend/internal/pkg/types"
)

func LoadRunResult(projectRoot, runID string) (types.RunResult, error) {
	dir := filepath.Join(projectRoot, "artifacts", runID)
	reportPath := filepath.Join(dir, "report.json")
	resultPath := filepath.Join(dir, "result.json")

	path := reportPath
	if _, err := os.Stat(path); err != nil {
		path = resultPath
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return types.RunResult{}, fmt.Errorf("run result not found for %s", runID)
	}

	var result types.RunResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return types.RunResult{}, err
	}
	if result.RunID == "" {
		result.RunID = runID
	}
	return result, nil
}

func SaveRunResult(projectRoot string, result types.RunResult) (types.RunResult, error) {
	reportPath, err := report.WriteRunReport(projectRoot, result)
	if err != nil {
		return result, err
	}
	result.Artifacts.ReportPath, _ = filepath.Abs(reportPath)
	if len(result.PromotionCandidates) > 0 {
		candidatePath := filepath.Join(projectRoot, "artifacts", result.RunID, "promotion-candidates.json")
		_ = respond.JSONFile(candidatePath, result.PromotionCandidates)
	}
	_, err = report.WriteRunReport(projectRoot, result)
	return result, err
}
