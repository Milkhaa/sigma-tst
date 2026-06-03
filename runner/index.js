import fs from 'fs';
import path from 'path';
import { chromium } from 'playwright';

const actionTimeoutMs = Number(process.env.PW_ACTION_TIMEOUT_MS || 5000);
const navigationTimeoutMs = Number(process.env.PW_NAVIGATION_TIMEOUT_MS || 10000);
const slowMoMs = Number(process.env.PW_SLOW_MO_MS || 3000);
const headless = process.env.PW_HEADLESS === 'true';

function arg(name) {
  const i = process.argv.indexOf(name);
  if (i === -1) return '';
  return process.argv[i + 1] || '';
}

async function run() {
  const specPath = arg('--spec');
  const outPath = arg('--out');
  const artifactDir = arg('--artifactDir');
  const runId = arg('--runId') || path.basename(artifactDir || '');
  if (!specPath || !outPath) {
    throw new Error('Missing --spec or --out');
  }

  const spec = JSON.parse(fs.readFileSync(specPath, 'utf-8'));
  const result = {
    runId,
    status: 'passed',
    steps: [],
    promotionCandidates: [],
    artifacts: {}
  };

  let recoveredAnyStep = false;
  const browser = await chromium.launch({ headless, slowMo: slowMoMs });
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    for (const step of spec.steps) {
      const started = Date.now();

      // Deterministic path (with optional LLM-based fallback recovery).
      try {
        await executeStep(page, spec, step);
        result.steps.push({
          stepId: step.id,
          action: step.action,
          mode: 'deterministic',
          status: 'passed',
          durationMs: Date.now() - started,
          message: 'ok',
          decisionTrace: deterministicTrace(step)
        });
      } catch (err) {
        if (step.allowRecovery) {
          const recoveryScreenshot = path.join(artifactDir || path.dirname(outPath), `${step.id}-before-recovery.png`);
          await page.screenshot({ path: recoveryScreenshot, fullPage: true });

          try {
            const recovery = await recoverStep(page, step);
            recoveredAnyStep = true;
            const decisionTrace = agenticTrace(step, err.message, recovery);
            const promotionCandidate = buildPromotionCandidate(runId, step, recovery, decisionTrace);
            result.promotionCandidates.push(promotionCandidate);
            result.steps.push({
              stepId: step.id,
              action: step.action,
              mode: 'agentic',
              status: 'recovered',
              durationMs: Date.now() - started,
              message: `deterministic selector failed; fallback recovered. original error: ${err.message}`,
              unverifiedRecovery: recovery.unverifiedRecovery || false,
              recovery: {
                ...recovery,
                failedSelector: step.selector,
                screenshotPath: recoveryScreenshot
              },
              decisionTrace,
              promotionCandidate
            });
            continue;
          } catch (recoveryErr) {
            const screenshot = path.join(artifactDir || path.dirname(outPath), 'failure.png');
            await page.screenshot({ path: screenshot, fullPage: true });
            result.status = 'failed';
            result.artifacts.failureScreenshot = screenshot;
            result.steps.push({
              stepId: step.id,
              action: step.action,
              mode: 'agentic',
              status: 'failed',
              durationMs: Date.now() - started,
              message: `deterministic selector failed and recovery failed. original error: ${err.message}. recovery error: ${recoveryErr.message}`,
              recovery: {
                intent: step.recovery?.intent || step.description || '',
                failedSelector: step.selector,
                strategy: 'llm-candidate-selection',
                screenshotPath: recoveryScreenshot,
                message: recoveryErr.message
              },
              decisionTrace: failedRecoveryTrace(step, err.message, recoveryErr.message)
            });
            break;
          }
        }

        const screenshot = path.join(artifactDir || path.dirname(outPath), 'failure.png');
        await page.screenshot({ path: screenshot, fullPage: true });
        result.status = 'failed';
        result.artifacts.failureScreenshot = screenshot;
        result.steps.push({
          stepId: step.id,
          action: step.action,
          mode: 'deterministic',
          status: 'failed',
          durationMs: Date.now() - started,
          message: err.message,
          decisionTrace: {
            ...deterministicTrace(step),
            failure: err.message,
            customerExplanation: `The deterministic ${step.action} step failed before recovery was allowed.`
          }
        });
        break;
      }
    }
  } finally {
    await browser.close();
  }

  if (result.status !== 'failed' && recoveredAnyStep) {
    result.status = 'recovered';
  }

  fs.writeFileSync(outPath, JSON.stringify(result, null, 2));
}

async function executeStep(page, spec, step) {
  if (step.action === 'goto') {
    const target = step.target?.startsWith('http') ? step.target : new URL(step.target, spec.baseUrl).toString();
    await page.goto(target, { waitUntil: 'domcontentloaded', timeout: navigationTimeoutMs });
    if (step.expect?.urlContains) {
      assertURLContains(page, step.expect.urlContains);
    }
    return;
  }

  if (step.action === 'click') {
    await clickSelector(page, step.selector);
    return;
  }

  if (step.action === 'fill') {
    await page.locator(step.selector).first().fill(step.value || '', { timeout: actionTimeoutMs });
    return;
  }

  if (step.action === 'press') {
    await page.locator(step.selector).first().press(step.key || 'Enter', { timeout: actionTimeoutMs });
    return;
  }

  if (step.action === 'waitFor') {
    await page.locator(step.selector).first().waitFor({ state: 'visible', timeout: actionTimeoutMs });
    return;
  }

  if (step.action === 'assert') {
    await executeAssertions(page, step.expect || {});
    return;
  }

  throw new Error(`unsupported action: ${step.action}`);
}

async function clickSelector(page, selector) {
  const locator = page.locator(selector).first();
  await locator.waitFor({ state: 'visible', timeout: actionTimeoutMs });
  await locator.scrollIntoViewIfNeeded({ timeout: actionTimeoutMs });
  await locator.click({ timeout: actionTimeoutMs });
}

async function executeAssertions(page, expect) {
  if (expect.urlContains) {
    assertURLContains(page, expect.urlContains);
  }
  if (expect.textVisible) {
    await page.getByText(expect.textVisible, { exact: false }).first().waitFor({ state: 'visible', timeout: actionTimeoutMs });
  }
  if (expect.elementVisible) {
    await page.locator(expect.elementVisible).first().waitFor({ state: 'visible', timeout: actionTimeoutMs });
  }
  if (expect.elementCount) {
    const count = await page.locator(expect.elementCount.selector).count();
    if (count !== expect.elementCount.count) {
      throw new Error(`elementCount failed for ${expect.elementCount.selector}: expected ${expect.elementCount.count}, got ${count}`);
    }
  }
  if (expect.valueEquals) {
    const val = await page.locator(expect.valueEquals.selector).first().inputValue();
    if (val !== expect.valueEquals.value) {
      throw new Error(`valueEquals failed for ${expect.valueEquals.selector}: expected ${expect.valueEquals.value}, got ${val}`);
    }
  }
}

function assertURLContains(page, expected) {
  const current = page.url();
  if (!current.includes(expected)) {
    throw new Error(`URL assertion failed. expected contains ${expected}, got ${current}`);
  }
}

// LLM-based recovery with heuristic fallback when no API key is present.
async function recoverStep(page, step) {
  if (step.action !== 'click') {
    throw new Error(`recovery is only implemented for click steps, got ${step.action}`);
  }

  const candidates = await collectClickableCandidates(page);
  const hasLLM = process.env.ANTHROPIC_API_KEY || process.env.OPENAI_API_KEY;
  const intent = step.recovery?.intent || step.description || '';
  const hasValidation = !!(step.recovery?.expectedUrlContains || step.recovery?.expectedText);

  let recoveredSelector, strategy, reasoning, customerExplanation;

  if (hasLLM) {
    const prompt = buildRecoveryPrompt(step, candidates);
    const llmResult = await callLLM(prompt);
    recoveredSelector = llmResult.selector;
    strategy = 'llm-candidate-selection';
    reasoning = llmResult.reasoning;
    customerExplanation = llmResult.customerExplanation;
  } else {
    const ranked = rankCandidates(candidates, step.recovery || {}, step.description || '');
    const best = ranked[0];
    if (!best || best.score <= 0) {
      throw new Error('no recovery candidate matched the step intent');
    }
    recoveredSelector = best.selector;
    strategy = 'page-state-candidate-ranking';
    reasoning = `The candidate "${best.text || best.href || best.selector}" matched the recovery intent with score ${best.score}.`;
    customerExplanation = `The saved selector was outdated, so the agent found a current page element for "${intent}" and validated the result.`;
  }

  await clickSelector(page, recoveredSelector);
  await validateRecoveryOutcome(page, step.recovery || {});

  if (!hasValidation) {
    customerExplanation += ' Warning: no outcome validation was configured — the agent clicked an element but could not confirm the expected result.';
  }

  return {
    intent,
    recoveredSelector,
    strategy,
    candidateCount: candidates.length,
    message: reasoning,
    reasoning,
    customerExplanation,
    evidence: recoveryEvidence(step.recovery || {}),
    unverifiedRecovery: !hasValidation
  };
}


function deterministicTrace(step) {
  const selector = step.selector || step.target || '';
  return {
    mode: 'deterministic',
    attemptedAction: step.action,
    attemptedSelector: selector,
    reasoning: `Ran the saved ${step.action} step with the deterministic selector or target.`,
    customerExplanation: `The saved ${step.action} step ran as expected.`,
    evidence: expectedEvidence(step.expect || {})
  };
}

function agenticTrace(step, failure, recovery) {
  return {
    mode: 'agentic',
    attemptedAction: step.action,
    attemptedSelector: step.selector || '',
    failure,
    agentDecision: 'click',
    selectedSelector: recovery.recoveredSelector,
    reasoning: recovery.reasoning || recovery.message,
    customerExplanation: recovery.customerExplanation || `The saved selector was outdated, so the agent found a current page element for "${recovery.intent || step.description}" and validated the result.`,
    evidence: recovery.evidence || recoveryEvidence(step.recovery || {})
  };
}

function failedRecoveryTrace(step, failure, recoveryFailure) {
  return {
    mode: 'agentic',
    attemptedAction: step.action,
    attemptedSelector: step.selector || '',
    failure,
    agentDecision: 'none',
    reasoning: `Recovery was attempted but did not find a validated candidate. ${recoveryFailure}`,
    customerExplanation: `The saved selector was outdated and the agent could not safely find a replacement.`,
    evidence: ''
  };
}

function buildPromotionCandidate(runId, step, recovery, decisionTrace) {
  return {
    id: `${runId}-${step.id}-promotion`,
    runId,
    stepId: step.id,
    status: 'pending',
    originalAction: step.action,
    originalSelector: step.selector || '',
    proposedAction: step.action,
    proposedSelector: recovery.recoveredSelector,
    reasoning: decisionTrace.reasoning,
    customerExplanation: decisionTrace.customerExplanation,
    evidence: decisionTrace.evidence
  };
}

function expectedEvidence(expect) {
  if (expect.urlContains) return `URL contains "${expect.urlContains}"`;
  if (expect.textVisible) return `Text "${expect.textVisible}" is visible`;
  if (expect.elementVisible) return `Element "${expect.elementVisible}" is visible`;
  if (expect.elementCount) return `Element count for "${expect.elementCount.selector}" equals ${expect.elementCount.count}`;
  if (expect.valueEquals) return `Value for "${expect.valueEquals.selector}" equals "${expect.valueEquals.value}"`;
  return '';
}

function recoveryEvidence(recovery) {
  const parts = [];
  if (recovery.expectedUrlContains) parts.push(`URL contains "${recovery.expectedUrlContains}"`);
  if (recovery.expectedText) parts.push(`Text "${recovery.expectedText}" is visible`);
  return parts.join('; ');
}

async function collectClickableCandidates(page) {
  const rawCandidates = await page.evaluate(() => {
    const isVisible = (el) => {
      const rect = el.getBoundingClientRect();
      const style = window.getComputedStyle(el);
      return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none' && style.opacity !== '0';
    };

    // Walk up to 8 ancestors to find a short, meaningful label for the surrounding context.
    const nearbyText = (el) => {
      let current = el.parentElement;
      let fallback = '';
      for (let depth = 0; current && depth < 8; depth++) {
        const text = (current.innerText || current.textContent || '').replace(/\s+/g, ' ').trim();
        if (text && !fallback) fallback = text.slice(0, 180);
        if (text.length >= 10 && text.length <= 160) return text;
        current = current.parentElement;
      }
      return fallback.slice(0, 180);
    };

    return Array.from(document.querySelectorAll('a[href], button, [role="button"], [role="link"]'))
      .filter(isVisible)
      .slice(0, 100)
      .map((el, index) => ({
        index,
        tag: el.tagName.toLowerCase(),
        role: el.getAttribute('role') || '',
        text: (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 160),
        ariaLabel: (el.getAttribute('aria-label') || '').trim().slice(0, 160),
        title: (el.getAttribute('title') || '').trim().slice(0, 160),
        href: el.href || el.getAttribute('href') || '',
        nearbyText: nearbyText(el)
      }));
  });

  // Count occurrences of each button text to know when :near() scoping is needed.
  const textCounts = new Map();
  for (const c of rawCandidates) {
    if (c.tag === 'button') textCounts.set(c.text, (textCounts.get(c.text) || 0) + 1);
  }
  const textOccurrence = new Map();

  return rawCandidates.map((candidate) => {
    let occurrence = 1;
    if (candidate.tag === 'button') {
      occurrence = (textOccurrence.get(candidate.text) || 0) + 1;
      textOccurrence.set(candidate.text, occurrence);
    }
    const totalCount = textCounts.get(candidate.text) || 1;
    return {
      ...candidate,
      selector: selectorForCandidate(candidate, occurrence, totalCount)
    };
  }).filter((candidate) => candidate.selector);
}

function rankCandidates(candidates, recovery, description) {
  const expectedUrl = normalize(recovery.expectedUrlContains || '');
  const expectedText = normalize(recovery.expectedText || '');
  const intentWords = keywords(`${recovery.intent || ''} ${description || ''} ${expectedUrl} ${expectedText}`);

  return candidates
    .map((candidate) => {
      const text = normalize(`${candidate.text} ${candidate.ariaLabel} ${candidate.title}`);
      const href = normalize(candidate.href);
      const selector = normalize(candidate.selector);
      const haystack = `${text} ${href} ${selector}`;
      let score = 0;

      if (expectedUrl && href.includes(expectedUrl)) score += 100;
      if (expectedUrl && selector.includes(expectedUrl)) score += 60;
      if (expectedText && text.includes(expectedText)) score += 70;

      for (const word of intentWords) {
        if (haystack.includes(word)) score += 15;
      }

      if (candidate.tag === 'a' && expectedUrl) score += 5;

      return { ...candidate, score };
    })
    .sort((a, b) => b.score - a.score);
}

function selectorForCandidate(candidate, occurrence = 1, totalCount = 1) {
  if (candidate.tag === 'a' && candidate.href) {
    const token = lastPathToken(candidate.href);
    if (token) return `a[href*="${attrEscape(token)}"]`;
  }

  const text = candidate.text || candidate.ariaLabel || candidate.title;
  if (text) {
    const tag = candidate.tag === 'button' ? 'button' : candidate.tag === 'a' ? 'a' : `[role="${attrEscape(candidate.role)}"]`;
    return `${tag}:has-text(${JSON.stringify(text)})`;
  }

  return '';
}

function lastPathToken(href) {
  try {
    const url = new URL(href, 'http://localhost');
    return url.pathname.split('/').filter(Boolean).pop() || '';
  } catch {
    return '';
  }
}

async function validateRecoveryOutcome(page, recovery) {
  const expectedUrl = recovery.expectedUrlContains || '';
  const expectedText = recovery.expectedText || '';

  if (expectedUrl) {
    await page.waitForFunction(
      (fragment) => window.location.href.includes(fragment),
      expectedUrl,
      { timeout: navigationTimeoutMs }
    );
  }

  if (expectedText) {
    await page.getByText(expectedText, { exact: false }).first().waitFor({ state: 'visible', timeout: navigationTimeoutMs });
  }
}

function normalize(value) {
  return String(value || '').toLowerCase().trim();
}

function keywords(value) {
  const stopWords = new Set(['the', 'and', 'with', 'open', 'click', 'go', 'to', 'page', 'verify', 'visible', 'link', 'button']);
  return Array.from(new Set(normalize(value).split(/[^a-z0-9]+/).filter((word) => word.length >= 3 && !stopWords.has(word))));
}

function attrEscape(value) {
  return String(value || '').replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

// ---- LLM integration ----

function buildRecoveryPrompt(step, candidates) {
  const intent = step.recovery?.intent || step.description || `${step.action} step`;
  const simplified = candidates.slice(0, 40).map((c) => ({
    selector: c.selector,
    text: c.text,
    ariaLabel: c.ariaLabel,
    href: c.href,
    tag: c.tag,
    nearbyText: c.nearbyText
  }));

  return `You are a Playwright test recovery agent. A test step failed because its saved selector is outdated due to UI drift.

Failed step:
- Action: ${step.action}
- Original (stale) selector: ${step.selector}
- Intent: ${intent}
- Expected URL after action contains: ${step.recovery?.expectedUrlContains || '(none)'}
- Expected text visible after action: ${step.recovery?.expectedText || '(none)'}

Current visible page elements. Each candidate includes a "nearbyText" field showing the surrounding DOM context:
${JSON.stringify(simplified, null, 2)}

Instructions:
- Use nearbyText to identify which candidate belongs to the intended item.
- If the same button text (e.g. "Add to cart") appears multiple times, construct a :near() selector using a distinctive word or phrase from the correct candidate's nearbyText. Choose a proximity distance that fits this site's layout.
- NEVER pick a generic unscoped selector (e.g. button:has-text("Add to cart") alone) when the intent targets a specific item — that would silently act on the wrong element.
- If no candidate can be confidently matched to the intent, explain why rather than guessing.

Return STRICT JSON only, no markdown, no prose:
{"selector":"<scoped Playwright selector>","reasoning":"<technical explanation>","customerExplanation":"<plain English for a non-technical reader>"}`;
}


async function callLLM(prompt) {
  const anthropicKey = process.env.ANTHROPIC_API_KEY || '';
  const openaiKey = process.env.OPENAI_API_KEY || '';

  if (anthropicKey) return callAnthropic(anthropicKey, prompt);
  if (openaiKey) return callOpenAI(openaiKey, prompt);
  throw new Error('No LLM API key configured (ANTHROPIC_API_KEY or OPENAI_API_KEY)');
}

async function callAnthropic(apiKey, prompt) {
  const response = await fetch('https://api.anthropic.com/v1/messages', {
    method: 'POST',
    headers: {
      'x-api-key': apiKey,
      'anthropic-version': '2023-06-01',
      'content-type': 'application/json'
    },
    body: JSON.stringify({
      model: 'claude-sonnet-4-6',
      max_tokens: 400,
      temperature: 0,
      messages: [{ role: 'user', content: prompt }]
    })
  });

  if (!response.ok) {
    const body = await response.text();
    throw new Error(`Anthropic API error ${response.status}: ${body}`);
  }

  const data = await response.json();
  return parseLLMJson(data.content[0].text);
}

async function callOpenAI(apiKey, prompt) {
  const model = process.env.OPENAI_MODEL || 'gpt-4o-mini';
  const response = await fetch('https://api.openai.com/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${apiKey}`,
      'content-type': 'application/json'
    },
    body: JSON.stringify({
      model,
      temperature: 0,
      messages: [
        { role: 'system', content: 'You output strict JSON only. No prose. No markdown.' },
        { role: 'user', content: prompt }
      ]
    })
  });

  if (!response.ok) {
    const body = await response.text();
    throw new Error(`OpenAI API error ${response.status}: ${body}`);
  }

  const data = await response.json();
  return parseLLMJson(data.choices[0].message.content);
}

function parseLLMJson(text) {
  text = String(text || '').trim();
  text = text.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/\s*```$/, '').trim();
  return JSON.parse(text);
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
