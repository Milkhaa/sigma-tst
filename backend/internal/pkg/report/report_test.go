package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sigma-tst/backend/internal/pkg/types"
)

func TestWriteRunReport_CreatesFile(t *testing.T) {
	root := t.TempDir()
	result := types.RunResult{RunID: "run-abc", Status: "passed"}

	path, err := WriteRunReport(root, result)
	require.NoError(t, err)

	expected := filepath.Join(root, "artifacts", "run-abc", "report.json")
	assert.Equal(t, expected, path)
	assert.FileExists(t, expected)
}

func TestWriteRunReport_ContentsRoundtrip(t *testing.T) {
	root := t.TempDir()
	result := types.RunResult{
		RunID:  "run-xyz",
		Status: "failed",
		Steps: []types.StepResult{
			{StepID: "s1", Action: "click", Status: "failed", DurationMs: 250},
		},
	}

	path, err := WriteRunReport(root, result)
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var got types.RunResult
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, result.RunID, got.RunID)
	assert.Equal(t, result.Status, got.Status)
	require.Len(t, got.Steps, 1)
	assert.Equal(t, "s1", got.Steps[0].StepID)
}

func TestWriteRunReport_OverwritesExisting(t *testing.T) {
	root := t.TempDir()
	result := types.RunResult{RunID: "run-dup", Status: "passed"}

	_, err := WriteRunReport(root, result)
	require.NoError(t, err)

	// Write again with updated status — second write must win
	result.Status = "failed"
	path, err := WriteRunReport(root, result)
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var got types.RunResult
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "failed", got.Status)
}
