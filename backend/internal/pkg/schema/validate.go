package schema

import (
	"fmt"
	"net/url"
	"strings"

	"sigma-tst/backend/internal/pkg/types"
)

var allowedActions = map[string]struct{}{
	"goto":    {},
	"click":   {},
	"fill":    {},
	"press":   {},
	"assert":  {},
	"waitFor": {},
}

func Validate(spec *types.TestSpec) []string {
	if spec == nil {
		return []string{"spec is required"}
	}
	var errs []string
	if strings.TrimSpace(spec.Name) == "" {
		errs = append(errs, "name is required")
	}
	if _, err := url.ParseRequestURI(spec.BaseURL); err != nil {
		errs = append(errs, "baseUrl must be a valid URL")
	}
	if len(spec.Steps) == 0 {
		errs = append(errs, "steps must not be empty")
	}
	for i, s := range spec.Steps {
		p := fmt.Sprintf("steps[%d]", i)
		if strings.TrimSpace(s.ID) == "" {
			errs = append(errs, p+".id is required")
		}
		if _, ok := allowedActions[s.Action]; !ok {
			errs = append(errs, p+".action is invalid")
		}
		if s.AllowRecovery {
			if s.Recovery == nil {
				errs = append(errs, p+".recovery is required when allowRecovery is true")
			} else if strings.TrimSpace(s.Recovery.Intent) == "" {
				errs = append(errs, p+".recovery.intent is required when allowRecovery is true")
			}
		}
		switch s.Action {
		case "goto":
			if strings.TrimSpace(s.Target) == "" {
				errs = append(errs, p+".target is required for goto")
			}
		case "click", "waitFor":
			if strings.TrimSpace(s.Selector) == "" {
				errs = append(errs, p+".selector is required")
			}
		case "fill":
			if strings.TrimSpace(s.Selector) == "" || s.Value == "" {
				errs = append(errs, p+".selector and value are required for fill")
			}
		case "press":
			if strings.TrimSpace(s.Selector) == "" || strings.TrimSpace(s.Key) == "" {
				errs = append(errs, p+".selector and key are required for press")
			}
		case "assert":
			if s.Expect == nil {
				errs = append(errs, p+".expect is required for assert")
			}
		}
	}
	return errs
}
