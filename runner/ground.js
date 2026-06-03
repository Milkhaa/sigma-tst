import fs from 'fs';
import { chromium } from 'playwright';

const actionTimeoutMs = Number(process.env.PW_ACTION_TIMEOUT_MS || 5000);
const navigationTimeoutMs = Number(process.env.PW_NAVIGATION_TIMEOUT_MS || 10000);
const headless = process.env.PW_HEADLESS !== 'false';

function arg(name) {
  const i = process.argv.indexOf(name);
  if (i === -1) return '';
  return process.argv[i + 1] || '';
}

async function run() {
  const specPath = arg('--spec');
  const outPath = arg('--out');
  if (!specPath || !outPath) throw new Error('Missing --spec or --out');

  const spec = JSON.parse(fs.readFileSync(specPath, 'utf-8'));
  const hasLLM = !!(process.env.ANTHROPIC_API_KEY || process.env.OPENAI_API_KEY);

  const browser = await chromium.launch({ headless });
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    for (const step of spec.steps || []) {

      // ── goto: navigate and continue ────────────────────────────────────────
      if (step.action === 'goto') {
        const target = step.target?.startsWith('http')
          ? step.target
          : new URL(step.target || '/', spec.baseUrl).toString();
        try {
          await page.goto(target, { waitUntil: 'domcontentloaded', timeout: navigationTimeoutMs });
          await page.waitForLoadState('networkidle', { timeout: navigationTimeoutMs }).catch(() => {});
        } catch (err) {
          step.ungrounded = true;
          step.groundingNote = `Navigation failed: ${err.message.split('\n')[0]}`;
        }
        continue;
      }

      // ── interactive steps: ground selector with LLM, then execute ──────────
      if (['click', 'fill', 'press', 'waitFor'].includes(step.action)) {
        const candidates = await collectCandidates(page);

        if (hasLLM) {
          try {
            const result = await callLLM(buildGroundingPrompt(step, candidates));
            if (result.selector) step.selector = result.selector;
            if (!result.confident) {
              step.ungrounded = true;
              step.groundingNote = `LLM uncertain: ${result.reasoning}`;
            }
          } catch (llmErr) {
            // LLM failed — fall back to keyword heuristic and flag the step
            const best = bestCandidateForStep(step, candidates);
            if (best?.suggestedSelectors?.length) step.selector = best.suggestedSelectors[0];
            step.ungrounded = true;
            step.groundingNote = `LLM grounding failed (${llmErr.message.split('\n')[0]}); heuristic selector used`;
          }
        } else {
          // No LLM key — heuristic only
          const best = bestCandidateForStep(step, candidates);
          if (best?.suggestedSelectors?.length) step.selector = best.suggestedSelectors[0];
        }

        // Execute the step so the browser advances to the correct page state
        // for the next step's grounding. Failure here is non-fatal but noted.
        try {
          await executeStep(page, spec, step);
          await page.waitForLoadState('networkidle', { timeout: navigationTimeoutMs }).catch(() => {});
        } catch (execErr) {
          const note = `Execution failed during grounding: ${execErr.message.split('\n')[0]}`;
          step.ungrounded = true;
          step.groundingNote = step.groundingNote ? `${step.groundingNote}; ${note}` : note;
        }
        continue;
      }

      // ── assert: validate conditions against the live page ──────────────────
      if (step.action === 'assert' && step.expect) {
        if (hasLLM) {
          const candidates = await collectCandidates(page);
          const url = page.url();
          const pageText = await getPageVisibleText(page);
          try {
            const result = await callLLM(buildAssertGroundingPrompt(step, candidates, url, pageText));
            if (!result.valid) {
              if (result.corrections && Object.keys(result.corrections).length > 0) {
                // Apply LLM-suggested corrections to the spec step
                if (result.corrections.textVisible != null) {
                  step.expect.textVisible = result.corrections.textVisible || undefined;
                }
                if (result.corrections.elementVisible != null) {
                  step.expect.elementVisible = result.corrections.elementVisible || undefined;
                }
                if (result.corrections.urlContains != null) {
                  step.expect.urlContains = result.corrections.urlContains || undefined;
                }
                step.groundingNote = `Assert corrected: ${result.reasoning}`;
              } else {
                step.ungrounded = true;
                step.groundingNote = `Assert could not be matched to page: ${result.reasoning}`;
              }
            }
          } catch {
            // Non-fatal: keep original assert conditions as-is
          }
        }
        continue;
      }
    }
  } finally {
    await browser.close();
  }

  fs.writeFileSync(outPath, JSON.stringify(spec, null, 2));
}

async function executeStep(page, spec, step) {
  if (step.action === 'click') {
    if (!step.selector) throw new Error('no selector');
    const locator = page.locator(step.selector).first();
    await locator.waitFor({ state: 'visible', timeout: actionTimeoutMs });
    await locator.scrollIntoViewIfNeeded({ timeout: actionTimeoutMs });
    await locator.click({ timeout: actionTimeoutMs });
    return;
  }
  if (step.action === 'fill') {
    if (!step.selector) throw new Error('no selector');
    await page.locator(step.selector).first().fill(step.value || '', { timeout: actionTimeoutMs });
    return;
  }
  if (step.action === 'press') {
    if (!step.selector) throw new Error('no selector');
    await page.locator(step.selector).first().press(step.key || 'Enter', { timeout: actionTimeoutMs });
    return;
  }
  if (step.action === 'waitFor') {
    if (!step.selector) throw new Error('no selector');
    await page.locator(step.selector).first().waitFor({ state: 'visible', timeout: actionTimeoutMs });
    return;
  }
}

async function getPageVisibleText(page) {
  return page.evaluate(() =>
    (document.body.innerText || document.body.textContent || '')
      .replace(/\s+/g, ' ').trim().slice(0, 1500)
  );
}

async function collectCandidates(page) {
  const snapshot = await page.evaluate(() => {
    const isVisible = (el) => {
      const rect = el.getBoundingClientRect();
      const style = window.getComputedStyle(el);
      return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none' && style.opacity !== '0';
    };

    const clean = (value) => String(value || '').replace(/\s+/g, ' ').trim().slice(0, 180);
    const nearbyText = (el) => {
      let current = el.parentElement;
      let fallback = '';
      for (let depth = 0; current && depth < 8; depth++) {
        const text = clean(current.innerText || current.textContent || '');
        if (text && !fallback) fallback = text;
        if (text.length >= 10 && text.length <= 160) return text;
        current = current.parentElement;
      }
      return fallback;
    };

    return Array.from(document.querySelectorAll('a[href], button, input, textarea, select, [role="button"], [role="link"]'))
      .filter(isVisible)
      .slice(0, 120)
      .map((el, index) => {
        const tag = el.tagName.toLowerCase();
        const text = clean(el.innerText || el.textContent || el.value || '');
        const ariaLabel = clean(el.getAttribute('aria-label'));
        const name = clean(el.getAttribute('name'));
        const placeholder = clean(el.getAttribute('placeholder'));
        const href = el.href || el.getAttribute('href') || '';
        const role = el.getAttribute('role') || (tag === 'button' ? 'button' : tag === 'a' ? 'link' : '');
        const label = ariaLabel || text || placeholder || name;
        return { index, tag, role, type: el.getAttribute('type') || '', text, label, name, placeholder, href, disabled: Boolean(el.disabled || el.getAttribute('aria-disabled') === 'true'), nearbyText: nearbyText(el) };
      });
  });

  return addSuggestedSelectors(snapshot);
}

function addSuggestedSelectors(candidates) {
  const totalCounts = new Map();
  for (const c of candidates) {
    const key = `${c.tag}:${c.text || c.label}`;
    totalCounts.set(key, (totalCounts.get(key) || 0) + 1);
  }
  const occurrenceCounts = new Map();
  return candidates.map((c) => {
    const key = `${c.tag}:${c.text || c.label}`;
    const occurrence = (occurrenceCounts.get(key) || 0) + 1;
    occurrenceCounts.set(key, occurrence);
    return { ...c, suggestedSelectors: suggestedSelectors(c, occurrence, totalCounts.get(key) || 0) };
  });
}

function suggestedSelectors(candidate, occurrence, totalCount) {
  const selectors = [];
  if (candidate.tag === 'a' && candidate.href) {
    const token = lastPathToken(candidate.href);
    if (token) selectors.push(`a[href*="${attrEscape(token)}"]`);
  }
  if (candidate.tag === 'button' && candidate.text) {
    if (totalCount > 1) selectors.push(`:nth-match(button:has-text(${JSON.stringify(candidate.text)}), ${occurrence})`);
    selectors.push(`button:has-text(${JSON.stringify(candidate.text)})`);
  }
  if (candidate.role && candidate.label) selectors.push(`${candidate.tag || '*'}:has-text(${JSON.stringify(candidate.label)})`);
  if (candidate.placeholder) selectors.push(`[placeholder="${attrEscape(candidate.placeholder)}"]`);
  if (candidate.name) selectors.push(`[name="${attrEscape(candidate.name)}"]`);
  return Array.from(new Set(selectors)).slice(0, 4);
}

function bestCandidateForStep(step, candidates) {
  const needle = normalize([step.target, step.description, step.selector].filter(Boolean).join(' '));
  const words = keywords(needle);
  let best = null;
  let bestScore = 0;

  for (const candidate of candidates) {
    if (!candidate.suggestedSelectors?.length) continue;
    const haystack = normalize([candidate.text, candidate.label, candidate.name, candidate.placeholder, candidate.href, candidate.nearbyText].filter(Boolean).join(' '));
    let score = candidate.disabled ? -40 : 0;
    for (const word of words) {
      if (haystack.includes(word)) score += 25;
      if (normalize(candidate.nearbyText).includes(word)) score += 15;
    }
    if (candidate.text && needle.includes(normalize(candidate.text))) score += 30;
    if (candidate.label && needle.includes(normalize(candidate.label))) score += 30;
    if (score > bestScore) { best = candidate; bestScore = score; }
  }
  return bestScore >= 25 ? best : null;
}

function keywords(value) {
  const stop = new Set(['click', 'open', 'the', 'and', 'to', 'on', 'complete', 'homepage', 'button', 'link', 'role', 'name', 'has', 'text']);
  return Array.from(new Set(normalize(value).split(/[^a-z0-9]+/).filter((w) => w.length >= 3 && !stop.has(w))));
}

function normalize(value) { return String(value || '').toLowerCase().trim(); }

function lastPathToken(href) {
  try { return new URL(href, 'http://localhost').pathname.split('/').filter(Boolean).pop() || ''; }
  catch { return ''; }
}

function attrEscape(value) { return String(value || '').replace(/\\/g, '\\\\').replace(/"/g, '\\"'); }

// ── LLM grounding prompts ───────────────────────────────────────────────────

function buildGroundingPrompt(step, candidates) {
  const simplified = candidates.slice(0, 40).map((c) => ({
    selector: c.suggestedSelectors?.[0] || '',
    text: c.text,
    label: c.label,
    placeholder: c.placeholder,
    name: c.name,
    href: c.href,
    tag: c.tag,
    nearbyText: c.nearbyText
  }));

  return `You are a test spec grounding agent. You have a draft test step and the current live page.
Confirm whether the draft selector uniquely targets the right element, or provide a better one.

Step:
- Action: ${step.action}
- Intent: ${step.description || step.action}
- Draft selector: ${step.selector || '(none)'}
${step.value ? `- Value to enter: ${step.value}` : ''}

Current page elements with surrounding DOM context (nearbyText):
${JSON.stringify(simplified, null, 2)}

Rules:
- If the draft selector already uniquely identifies the correct element, return it with confident:true.
- If the same element text appears on multiple elements (e.g. multiple "Add to cart" buttons), construct a scoped :near() selector using a distinctive word from the matching candidate's nearbyText and an appropriate distance for this site's layout.
- If you cannot confidently identify the correct element, return the best attempt with confident:false and explain why.

Return STRICT JSON only, no markdown:
{"selector":"<playwright selector>","confident":<true|false>,"reasoning":"<explanation>"}`;
}

function buildAssertGroundingPrompt(step, candidates, url, pageText) {
  const simplified = candidates.slice(0, 30).map((c) => ({
    selector: c.suggestedSelectors?.[0] || '',
    text: c.text,
    label: c.label,
    href: c.href,
    tag: c.tag
  }));

  return `You are a test spec grounding agent. Check whether an assert step's conditions match the current live page.

Current page URL: ${url}
Current page visible text (first 1500 chars): ${pageText}

Assert step conditions:
${JSON.stringify(step.expect || {}, null, 2)}

Current page visible elements:
${JSON.stringify(simplified, null, 2)}

For each condition, check the real page:
- textVisible: is this exact text (or very close) present in the page text? If close but wrong, suggest the correct value.
- elementVisible: does this selector match a visible element on the page?
- urlContains: does the current URL contain this fragment?

Return STRICT JSON only, no markdown:
{
  "valid": <true if all conditions match as-is>,
  "corrections": <object with corrected values for mismatched conditions, null if no corrections>,
  "reasoning": "<what matched, what didn't, and why>"
}`;
}

// ── LLM call infrastructure (mirrors runner/index.js) ───────────────────────

async function callLLM(prompt) {
  const anthropicKey = process.env.ANTHROPIC_API_KEY || '';
  const openaiKey = process.env.OPENAI_API_KEY || '';
  if (anthropicKey) return callAnthropic(anthropicKey, prompt);
  if (openaiKey) return callOpenAI(openaiKey, prompt);
  throw new Error('No LLM API key configured');
}

async function callAnthropic(apiKey, prompt) {
  const response = await fetch('https://api.anthropic.com/v1/messages', {
    method: 'POST',
    headers: { 'x-api-key': apiKey, 'anthropic-version': '2023-06-01', 'content-type': 'application/json' },
    body: JSON.stringify({ model: 'claude-sonnet-4-6', max_tokens: 400, temperature: 0, messages: [{ role: 'user', content: prompt }] })
  });
  if (!response.ok) { const b = await response.text(); throw new Error(`Anthropic ${response.status}: ${b}`); }
  const data = await response.json();
  return parseLLMJson(data.content[0].text);
}

async function callOpenAI(apiKey, prompt) {
  const model = process.env.OPENAI_MODEL || 'gpt-4o-mini';
  const response = await fetch('https://api.openai.com/v1/chat/completions', {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${apiKey}`, 'content-type': 'application/json' },
    body: JSON.stringify({ model, temperature: 0, messages: [{ role: 'system', content: 'Output strict JSON only.' }, { role: 'user', content: prompt }] })
  });
  if (!response.ok) { const b = await response.text(); throw new Error(`OpenAI ${response.status}: ${b}`); }
  const data = await response.json();
  return parseLLMJson(data.choices[0].message.content);
}

function parseLLMJson(text) {
  text = String(text || '').trim().replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/\s*```$/, '').trim();
  return JSON.parse(text);
}

run().catch((err) => { console.error(err); process.exit(1); });
