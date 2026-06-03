package schema

import (
	"regexp"
	"strings"

	"sigma-tst/backend/internal/pkg/types"
)

var roleSelectorRE = regexp.MustCompile(`role=(button|link|textbox)\[name=['"]([^'"]+)['"]\]`)

func NormalizeSpec(spec *types.TestSpec) {
	if spec == nil {
		return
	}
	for i := range spec.Steps {
		spec.Steps[i].Selector = NormalizeSelector(spec.Steps[i].Selector)
	}
}

func NormalizeSelector(selector string) string {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return ""
	}

	parts := splitSelectorList(selector)
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if converted := convertRoleSelector(part); converted != "" {
			normalized = append(normalized, converted)
			continue
		}
		normalized = append(normalized, part)
	}
	return strings.Join(normalized, ", ")
}

func convertRoleSelector(selector string) string {
	matches := roleSelectorRE.FindStringSubmatch(selector)
	if len(matches) != 3 {
		return ""
	}
	role := matches[1]
	name := matches[2]
	switch role {
	case "button":
		return `button:has-text("` + escapeSelectorText(name) + `")`
	case "link":
		return `a:has-text("` + escapeSelectorText(name) + `")`
	case "textbox":
		return `[placeholder="` + escapeSelectorText(name) + `"], [name="` + escapeSelectorText(name) + `"]`
	default:
		return ""
	}
}

func splitSelectorList(selector string) []string {
	var parts []string
	var current strings.Builder
	var quote rune
	depth := 0
	for _, r := range selector {
		if quote != 0 {
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			current.WriteRune(r)
		case '(':
			depth++
			current.WriteRune(r)
		case ')':
			if depth > 0 {
				depth--
			}
			current.WriteRune(r)
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}
	parts = append(parts, current.String())
	return parts
}

func escapeSelectorText(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}
