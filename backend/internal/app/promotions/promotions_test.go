package promotions

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sigma-tst/backend/internal/pkg/types"
)

func TestParsePath(t *testing.T) {
	testCases := []struct {
		name           string
		path           string
		expectedID     string
		expectedAction string
		expectedOK     bool
	}{
		{
			// Standard approve path
			name:           "approve path",
			path:           "/v1/promotions/abc-123/approve",
			expectedID:     "abc-123",
			expectedAction: "approve",
			expectedOK:     true,
		},
		{
			// Standard reject path
			name:           "reject path",
			path:           "/v1/promotions/xyz/reject",
			expectedID:     "xyz",
			expectedAction: "reject",
			expectedOK:     true,
		},
		{
			// Extra path segment makes it invalid
			name:       "too many segments",
			path:       "/v1/promotions/abc/approve/extra",
			expectedOK: false,
		},
		{
			// Empty candidate ID segment is rejected
			name:       "empty candidate id",
			path:       "/v1/promotions//approve",
			expectedOK: false,
		},
		{
			// Trailing slash after action gives an empty action segment
			name:       "trailing slash after action",
			path:       "/v1/promotions/abc/",
			expectedOK: false,
		},
		{
			// Prefix with only one segment is not a promotions path
			name:       "only prefix no segments",
			path:       "/v1/promotions/",
			expectedOK: false,
		},
		{
			// Unrelated route is not a promotions path
			name:       "unrelated route",
			path:       "/v1/tests/run",
			expectedOK: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id, action, ok := parsePath(tc.path)

			assert.Equal(t, tc.expectedOK, ok)
			if tc.expectedOK {
				assert.Equal(t, tc.expectedID, id)
				assert.Equal(t, tc.expectedAction, action)
			}
		})
	}
}

func TestFindCandidate(t *testing.T) {
	testCases := []struct {
		name          string
		candidates    []types.PromotionCandidate
		lookupID      string
		expectedStep  string
		expectedError error
	}{
		{
			// Candidate found by ID returns the correct record
			name: "found by id",
			candidates: []types.PromotionCandidate{
				{ID: "c1", StepID: "s1"},
				{ID: "c2", StepID: "s2"},
			},
			lookupID:     "c2",
			expectedStep: "s2",
		},
		{
			// ID not present in the list returns an error
			name:          "not found",
			candidates:    []types.PromotionCandidate{},
			lookupID:      "missing",
			expectedError: errors.New("promotion candidate not found: missing"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := &types.RunResult{PromotionCandidates: tc.candidates}
			candidate, err := findCandidate(result, tc.lookupID)

			if tc.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedStep, candidate.StepID)
			}
		})
	}
}

func TestSyncCandidate(t *testing.T) {
	testCases := []struct {
		name           string
		steps          []types.StepResult
		updated        *types.PromotionCandidate
		expectStatuses []string // expected PromotionCandidate.Status per step (empty string for nil)
	}{
		{
			// The matching step's candidate is replaced with the updated value
			name: "updates matching step",
			steps: []types.StepResult{
				{StepID: "s1", PromotionCandidate: &types.PromotionCandidate{ID: "c1", Status: "pending"}},
				{StepID: "s2"},
			},
			updated:        &types.PromotionCandidate{ID: "c1", Status: "approved"},
			expectStatuses: []string{"approved", ""},
		},
		{
			// Steps with nil PromotionCandidate are skipped without panic
			name: "nil promotion candidate is skipped",
			steps: []types.StepResult{
				{StepID: "s1", PromotionCandidate: nil},
			},
			updated:        &types.PromotionCandidate{ID: "c1", Status: "approved"},
			expectStatuses: []string{""},
		},
		{
			// A candidate ID that matches no step leaves all steps unchanged
			name: "no match leaves steps unchanged",
			steps: []types.StepResult{
				{StepID: "s1", PromotionCandidate: &types.PromotionCandidate{ID: "c99", Status: "pending"}},
			},
			updated:        &types.PromotionCandidate{ID: "c1", Status: "approved"},
			expectStatuses: []string{"pending"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := &types.RunResult{Steps: tc.steps}
			syncCandidate(result, tc.updated)

			for i, expected := range tc.expectStatuses {
				if expected == "" {
					assert.Nil(t, result.Steps[i].PromotionCandidate)
				} else {
					require.NotNil(t, result.Steps[i].PromotionCandidate)
					assert.Equal(t, expected, result.Steps[i].PromotionCandidate.Status)
				}
			}
		})
	}
}
