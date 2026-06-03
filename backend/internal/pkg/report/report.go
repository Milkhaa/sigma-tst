package report

import (
	"encoding/json"
	"os"
	"path/filepath"

	"sigma-tst/backend/internal/pkg/types"
)

func WriteRunReport(root string, result types.RunResult) (string, error) {
	dir := filepath.Join(root, "artifacts", result.RunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "report.json")
	b, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
