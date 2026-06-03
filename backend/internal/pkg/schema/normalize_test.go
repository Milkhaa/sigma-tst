package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"sigma-tst/backend/internal/pkg/types"
)

func TestNormalizeSelector(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			// Empty input should pass through unchanged
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			// Whitespace-only input collapses to empty
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			// Plain CSS selectors are not touched
			name:     "plain css selector unchanged",
			input:    "#submit-btn",
			expected: "#submit-btn",
		},
		{
			// Leading/trailing whitespace is trimmed
			name:     "trims outer whitespace",
			input:    "  #submit-btn  ",
			expected: "#submit-btn",
		},
		{
			// role=button[name='...'] → button:has-text("...")
			name:     "role button converted",
			input:    `role=button[name='Sign In']`,
			expected: `button:has-text("Sign In")`,
		},
		{
			// role=link[name='...'] → a:has-text("...")
			name:     "role link converted",
			input:    `role=link[name='Home']`,
			expected: `a:has-text("Home")`,
		},
		{
			// role=textbox[name='...'] → two selector alternatives
			name:     "role textbox converted",
			input:    `role=textbox[name='Email']`,
			expected: `[placeholder="Email"], [name="Email"]`,
		},
		{
			// Double-quoted name attribute works the same way
			name:     "role button with double quotes",
			input:    `role=button[name="Submit"]`,
			expected: `button:has-text("Submit")`,
		},
		{
			// Comma-separated list: role part is converted, plain CSS part is kept
			name:     "comma-separated list with role and css",
			input:    `role=button[name='OK'], #fallback`,
			expected: `button:has-text("OK"), #fallback`,
		},
		{
			// All-CSS comma list passes through intact
			name:     "comma-separated all css unchanged",
			input:    `.foo, .bar`,
			expected: `.foo, .bar`,
		},
		{
			// regex [^'"]+ excludes embedded quotes — selector passes through unchanged
			name:     "name containing embedded double-quote passes through",
			input:    `role=button[name='Say "hi"']`,
			expected: `role=button[name='Say "hi"']`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, NormalizeSelector(tc.input))
		})
	}
}

func TestNormalizeSpec_NilSpecIsNoop(t *testing.T) {
	// Must not panic
	NormalizeSpec(nil)
}

func TestNormalizeSpec_SelectorsInStepsAreNormalized(t *testing.T) {
	spec := &types.TestSpec{
		Name:    "test",
		BaseURL: "https://example.com",
		Steps: []types.Step{
			{ID: "1", Action: "click", Selector: `role=button[name='Go']`},
			{ID: "2", Action: "click", Selector: "#btn"},
		},
	}
	NormalizeSpec(spec)

	assert.Equal(t, `button:has-text("Go")`, spec.Steps[0].Selector)
	assert.Equal(t, "#btn", spec.Steps[1].Selector)
}

func TestSplitSelectorList(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			// Simple comma-separated values split at each comma
			name:     "simple comma list",
			input:    "a, b, c",
			expected: []string{"a", " b", " c"},
		},
		{
			// Comma inside quotes is NOT a split point
			name:     "comma inside double quotes not split",
			input:    `[name="a,b"], .c`,
			expected: []string{`[name="a,b"]`, " .c"},
		},
		{
			// Comma inside :is(...) parentheses is NOT a split point
			name:     "comma inside parens not split",
			input:    `:is(a, b), .c`,
			expected: []string{`:is(a, b)`, " .c"},
		},
		{
			// Single selector with no commas returns one-element slice
			name:     "single selector",
			input:    "single",
			expected: []string{"single"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, splitSelectorList(tc.input))
		})
	}
}

func TestEscapeSelectorText(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "double quotes are escaped",
			input:    `he said "hi"`,
			expected: `he said \"hi\"`,
		},
		{
			name:     "no quotes unchanged",
			input:    "no quotes",
			expected: "no quotes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, escapeSelectorText(tc.input))
		})
	}
}
