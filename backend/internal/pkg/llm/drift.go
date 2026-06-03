package llm

import (
	"fmt"
	"strings"

	"sigma-tst/backend/internal/pkg/types"
)

func InjectDriftFallbackDemo(spec *types.TestSpec) {
	if spec == nil {
		return
	}

	for i := range spec.Steps {
		step := &spec.Steps[i]
		if step.Action != "click" {
			continue
		}
		markStepAsDrifted(step)
		return
	}
}

func markStepAsDrifted(step *types.Step) {
	originalSelector := strings.TrimSpace(step.Selector)
	if originalSelector == "" {
		originalSelector = "unknown-selector"
	}

	intent := strings.TrimSpace(step.Description)
	if intent == "" {
		intent = fmt.Sprintf("Recover the intended %s step", step.Action)
	}

	step.Description = strings.TrimSpace(step.Description)
	step.Selector = fmt.Sprintf("[data-testid='stale-%s']", sanitizeSelectorToken(step.ID))
	step.AllowRecovery = true
	if step.Recovery == nil {
		step.Recovery = &types.RecoveryHint{}
	}
	if strings.TrimSpace(step.Recovery.Intent) == "" {
		step.Recovery.Intent = intent
	}
	if strings.TrimSpace(step.Recovery.ExpectedText) == "" {
		step.Recovery.ExpectedText = inferExpectedText(intent, originalSelector)
	}
}

func inferExpectedText(intent, selector string) string {
	source := strings.ToLower(intent + " " + selector)
	for _, word := range []string{"buy", "checkout", "purchase", "add", "login", "sign in", "cart"} {
		if strings.Contains(source, word) {
			return strings.Title(word)
		}
	}
	return ""
}

func sanitizeSelectorToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "step"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('-')
	}
	return strings.Trim(b.String(), "-")
}
