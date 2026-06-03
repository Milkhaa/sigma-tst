package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sigma-tst/backend/internal/pkg/types"
)

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	b, _ := json.MarshalIndent(v, "", "  ")
	require.NoError(t, os.WriteFile(path, b, 0o644))
}

func TestLoadRunResult(t *testing.T) {
	testCases := []struct {
		name     string
		setup    func(root string)
		runID    string
		validate func(t *testing.T, result types.RunResult, err error)
	}{
		{
			// report.json is the primary file; it should be loaded first
			name:  "loads from report.json",
			runID: "run-1",
			setup: func(root string) {
				writeJSON(t, filepath.Join(root, "artifacts", "run-1", "report.json"),
					types.RunResult{RunID: "run-1", Status: "passed"})
			},
			validate: func(t *testing.T, result types.RunResult, err error) {
				require.NoError(t, err)
				assert.Equal(t, "run-1", result.RunID)
				assert.Equal(t, "passed", result.Status)
			},
		},
		{
			// When report.json is absent, result.json is the fallback
			name:  "falls back to result.json when report.json missing",
			runID: "run-2",
			setup: func(root string) {
				writeJSON(t, filepath.Join(root, "artifacts", "run-2", "result.json"),
					types.RunResult{RunID: "run-2", Status: "failed"})
			},
			validate: func(t *testing.T, result types.RunResult, err error) {
				require.NoError(t, err)
				assert.Equal(t, "failed", result.Status)
			},
		},
		{
			// RunID missing from the file is backfilled from the directory name
			name:  "backfills RunID when absent from file",
			runID: "run-3",
			setup: func(root string) {
				writeJSON(t, filepath.Join(root, "artifacts", "run-3", "report.json"),
					map[string]string{"status": "passed"})
			},
			validate: func(t *testing.T, result types.RunResult, err error) {
				require.NoError(t, err)
				assert.Equal(t, "run-3", result.RunID)
			},
		},
		{
			// Missing run directory returns an error
			name:  "returns error for unknown runID",
			runID: "no-such-run",
			setup: func(root string) {},
			validate: func(t *testing.T, result types.RunResult, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(root)
			result, err := LoadRunResult(root, tc.runID)
			tc.validate(t, result, err)
		})
	}
}

func TestSaveRunResult_PersistsAndReturnsReportPath(t *testing.T) {
	root := t.TempDir()
	result := types.RunResult{RunID: "run-save", Status: "passed"}

	saved, err := SaveRunResult(root, result)
	require.NoError(t, err)
	assert.NotEmpty(t, saved.Artifacts.ReportPath)

	// Round-trip: the saved file must be loadable with correct content
	loaded, err := LoadRunResult(root, "run-save")
	require.NoError(t, err)
	assert.Equal(t, "passed", loaded.Status)
}

func TestSaveRunResult_WritesCandidateFile(t *testing.T) {
	root := t.TempDir()
	result := types.RunResult{
		RunID:  "run-cands",
		Status: "passed",
		PromotionCandidates: []types.PromotionCandidate{
			{ID: "c1", StepID: "s1", ProposedAction: "click", ProposedSelector: "#btn", Status: "pending"},
		},
	}

	_, err := SaveRunResult(root, result)
	require.NoError(t, err)

	// Candidates file must exist alongside the report
	candidatePath := filepath.Join(root, "artifacts", "run-cands", "promotion-candidates.json")
	assert.FileExists(t, candidatePath)
}
