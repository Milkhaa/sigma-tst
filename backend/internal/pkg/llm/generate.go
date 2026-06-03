package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"sigma-tst/backend/internal/pkg/inspect"
	"sigma-tst/backend/internal/pkg/types"
)

type anthropicResp struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type openAIResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func GenerateSpec(ctx context.Context, intent string, snapshot *inspect.Snapshot) (*types.TestSpec, bool, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	if provider == "openai" {
		return generateWithOpenAI(ctx, intent, snapshot)
	}
	if provider == "anthropic" {
		return generateWithAnthropic(ctx, intent, snapshot)
	}

	// Auto mode: prefer OpenAI when present, otherwise Anthropic.
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		return generateWithOpenAI(ctx, intent, snapshot)
	}
	if strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "" {
		return generateWithAnthropic(ctx, intent, snapshot)
	}
	return nil, false, errors.New("no LLM provider configured: set ANTHROPIC_API_KEY or OPENAI_API_KEY")
}

func generateWithAnthropic(ctx context.Context, intent string, snapshot *inspect.Snapshot) (*types.TestSpec, bool, error) {
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		return nil, false, errors.New("ANTHROPIC_API_KEY is not set")
	}

	prompt := buildPrompt(intent, snapshot)
	payload := map[string]any{
		"model":       "claude-sonnet-4-6",
		"max_tokens":  1200,
		"temperature": 0,
		"messages": []map[string]string{{
			"role":    "user",
			"content": prompt,
		}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("anthropic error: %s", string(raw))
	}

	var parsed anthropicResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, false, err
	}
	if len(parsed.Content) == 0 {
		return nil, false, errors.New("empty response content")
	}

	text := strings.TrimSpace(parsed.Content[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var spec types.TestSpec
	if err := json.Unmarshal([]byte(text), &spec); err != nil {
		return nil, false, err
	}
	return &spec, false, nil
}

func generateWithOpenAI(ctx context.Context, intent string, snapshot *inspect.Snapshot) (*types.TestSpec, bool, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return nil, false, errors.New("OPENAI_API_KEY is not set")
	}

	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = "gpt-4o-mini"
	}

	prompt := buildPrompt(intent, snapshot)
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You output strict JSON only. No prose. No markdown."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("openai error: %s", string(raw))
	}

	var parsed openAIResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, false, err
	}
	if len(parsed.Choices) == 0 {
		return nil, false, errors.New("empty openai response choices")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var spec types.TestSpec
	if err := json.Unmarshal([]byte(text), &spec); err != nil {
		return nil, false, err
	}
	return &spec, false, nil
}

func buildPrompt(intent string, snapshot *inspect.Snapshot) string {
	pageContext := pageContextForPrompt(snapshot)
	return fmt.Sprintf(`
## ROLE
You are a test planner. Output STRICT JSON only — no prose, no markdown, no explanation.

## GENERAL RULES
- Derive baseUrl from the URL in the intent. Localhost URLs are allowed.
- Allowed actions: goto, click, fill, press, assert, waitFor.
- Do not invent steps for targets not mentioned in the intent.
- Use valid Playwright selectors: button:has-text("Buy"), a[href*="path"], [name="field"], [placeholder="Email"].
  Do NOT use role=button[name=...] syntax.
- A disabled element may still be a valid selector for a later step if an earlier step enables it.

## SELECTOR RULES
1. NEVER use :nth-match() or any positional selector for a step that targets a specific named item.
   Positional selectors break silently when order changes.
2. Each page context candidate includes a nearbyText field (surrounding DOM text).
   When the same control appears multiple times, use nearbyText to build a scoped :near() selector.
   Choose a distinctive anchor word from nearbyText and pick a distance appropriate for this site's layout.
   Example: button:has-text("Add to cart"):near(:text("Burger"), 150)
   Do not assume a fixed distance — reason about the likely spacing on this page.
3. When page context is provided, start from the candidate's suggestedSelectors.
   Replace any :nth-match() entries with :near() selectors built from nearbyText.

## RECOVERY RULES
4. For every click step that targets a specific named item, set allowRecovery:true and populate:
   - recovery.intent: plain English description of what to find and do.
   - recovery.expectedText: distinctive text visible in that element's context that proves the right element was acted on
     (not text that appears anywhere on the page).
5. After every click on a specific named item, add an immediate assert step with expect.textVisible set to
   text that only appears if the correct item was acted on (e.g. item name in a cart or confirmation screen,
   not in the product listing). This catches silent wrong-element clicks.

## OUTPUT SCHEMA
%s

## PAGE CONTEXT
%s

## TASK
Intent: %s
`, schemaExample, pageContext, intent)
}

const schemaExample = `{
  "name": "string",
  "baseUrl": "string",
  "steps": [
    {
      "id": "step-1",
      "description": "optional human-readable description",
      "action": "goto | click | fill | press | assert | waitFor",
      "target": "URL or path (goto only)",
      "selector": "Playwright selector string",
      "value": "text to type (fill only)",
      "key": "key name (press only)",
      "allowRecovery": true,
      "recovery": {
        "intent": "plain English: what to find and do",
        "expectedUrlContains": "url fragment after action",
        "expectedText": "text visible near the element after action"
      },
      "expect": {
        "urlContains": "url fragment",
        "textVisible": "text that must be on page",
        "elementVisible": "selector that must be visible",
        "elementCount": { "selector": "css", "count": 1 },
        "valueEquals": { "selector": "css", "value": "expected" }
      }
    }
  ]
}`

func pageContextForPrompt(snapshot *inspect.Snapshot) string {
	if snapshot == nil {
		return "{}"
	}
	limited := *snapshot
	if len(limited.Candidates) > 40 {
		limited.Candidates = limited.Candidates[:40]
	}
	raw, err := json.Marshal(limited)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

