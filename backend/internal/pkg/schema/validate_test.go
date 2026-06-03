package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sigma-tst/backend/internal/pkg/types"
)

func validSpec() types.TestSpec {
	return types.TestSpec{
		Name:    "Login test",
		BaseURL: "https://example.com",
		Steps: []types.Step{
			{ID: "s1", Action: "goto", Target: "https://example.com/login"},
		},
	}
}

func TestValidate_NilSpec(t *testing.T) {
	errs := Validate(nil)
	require.Len(t, errs, 1)
	assert.Equal(t, "spec is required", errs[0])
}

func TestValidate_ValidSpec(t *testing.T) {
	spec := validSpec()
	assert.Empty(t, Validate(&spec))
}

func TestValidate_TopLevelFields(t *testing.T) {
	testCases := []struct {
		name          string
		mutate        func(*types.TestSpec)
		expectedError string
	}{
		{
			// Name must not be blank or whitespace
			name:          "missing name",
			mutate:        func(s *types.TestSpec) { s.Name = "   " },
			expectedError: "name is required",
		},
		{
			// BaseURL must parse as a valid URL
			name:          "invalid base url",
			mutate:        func(s *types.TestSpec) { s.BaseURL = "not-a-url" },
			expectedError: "baseUrl must be a valid URL",
		},
		{
			// Steps slice must not be nil or empty
			name:          "empty steps",
			mutate:        func(s *types.TestSpec) { s.Steps = nil },
			expectedError: "steps must not be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec()
			tc.mutate(&spec)
			assert.Contains(t, Validate(&spec), tc.expectedError)
		})
	}
}

func TestValidate_StepFields(t *testing.T) {
	testCases := []struct {
		name          string
		step          types.Step
		expectedError string
	}{
		{
			// Step ID is mandatory
			name:          "missing step id",
			step:          types.Step{ID: "", Action: "goto", Target: "https://example.com"},
			expectedError: "steps[0].id is required",
		},
		{
			// Only the six allowed actions are accepted
			name:          "invalid action",
			step:          types.Step{ID: "s1", Action: "hover"},
			expectedError: "steps[0].action is invalid",
		},
		{
			// goto requires a non-blank target
			name:          "goto missing target",
			step:          types.Step{ID: "s1", Action: "goto", Target: ""},
			expectedError: "steps[0].target is required for goto",
		},
		{
			// click requires a non-blank selector
			name:          "click missing selector",
			step:          types.Step{ID: "s1", Action: "click"},
			expectedError: "steps[0].selector is required",
		},
		{
			// waitFor requires a non-blank selector
			name:          "waitFor missing selector",
			step:          types.Step{ID: "s1", Action: "waitFor"},
			expectedError: "steps[0].selector is required",
		},
		{
			// fill requires both selector and value
			name:          "fill missing selector",
			step:          types.Step{ID: "s1", Action: "fill", Value: "hello"},
			expectedError: "steps[0].selector and value are required for fill",
		},
		{
			// fill with selector but no value also fails
			name:          "fill missing value",
			step:          types.Step{ID: "s1", Action: "fill", Selector: "#email"},
			expectedError: "steps[0].selector and value are required for fill",
		},
		{
			// press requires both selector and key
			name:          "press missing selector",
			step:          types.Step{ID: "s1", Action: "press", Key: "Enter"},
			expectedError: "steps[0].selector and key are required for press",
		},
		{
			// press with selector but no key also fails
			name:          "press missing key",
			step:          types.Step{ID: "s1", Action: "press", Selector: "#field"},
			expectedError: "steps[0].selector and key are required for press",
		},
		{
			// assert requires an expect block
			name:          "assert missing expect",
			step:          types.Step{ID: "s1", Action: "assert"},
			expectedError: "steps[0].expect is required for assert",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec()
			spec.Steps = []types.Step{tc.step}
			assert.Contains(t, Validate(&spec), tc.expectedError)
		})
	}
}

func TestValidate_ValidStepVariants(t *testing.T) {
	testCases := []struct {
		name string
		step types.Step
	}{
		{
			name: "fill with selector and value",
			step: types.Step{ID: "s1", Action: "fill", Selector: "#email", Value: "a@b.com"},
		},
		{
			name: "press with selector and key",
			step: types.Step{ID: "s1", Action: "press", Selector: "#field", Key: "Enter"},
		},
		{
			name: "assert with expect block",
			step: types.Step{ID: "s1", Action: "assert", Expect: &types.Expectation{URLContains: "/home"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec()
			spec.Steps = []types.Step{tc.step}
			assert.Empty(t, Validate(&spec))
		})
	}
}

func TestValidate_AllowRecovery(t *testing.T) {
	testCases := []struct {
		name          string
		step          types.Step
		expectedError error
	}{
		{
			// allowRecovery=true without a recovery block is invalid
			name: "allowRecovery without recovery block",
			step: types.Step{ID: "s1", Action: "click", Selector: "#btn", AllowRecovery: true},
			expectedError: errors.New("steps[0].recovery is required when allowRecovery is true"),
		},
		{
			// recovery block present but intent is blank is invalid
			name: "allowRecovery with blank intent",
			step: types.Step{
				ID: "s1", Action: "click", Selector: "#btn",
				AllowRecovery: true,
				Recovery:      &types.RecoveryHint{Intent: "  "},
			},
			expectedError: errors.New("steps[0].recovery.intent is required when allowRecovery is true"),
		},
		{
			// recovery block with a non-blank intent is valid
			name: "allowRecovery with valid intent",
			step: types.Step{
				ID: "s1", Action: "click", Selector: "#btn",
				AllowRecovery: true,
				Recovery:      &types.RecoveryHint{Intent: "click submit"},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec()
			spec.Steps = []types.Step{tc.step}
			errs := Validate(&spec)

			if tc.expectedError != nil {
				assert.Contains(t, errs, tc.expectedError.Error())
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}
