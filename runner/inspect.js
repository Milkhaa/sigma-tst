import fs from 'fs';
import { chromium } from 'playwright';

const navigationTimeoutMs = Number(process.env.PW_NAVIGATION_TIMEOUT_MS || 10000);
const headless = process.env.PW_HEADLESS !== 'false';

function arg(name) {
  const i = process.argv.indexOf(name);
  if (i === -1) return '';
  return process.argv[i + 1] || '';
}

async function run() {
  const url = arg('--url');
  const outPath = arg('--out');
  if (!url || !outPath) {
    throw new Error('Missing --url or --out');
  }

  const browser = await chromium.launch({ headless });
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: navigationTimeoutMs });
    await page.waitForLoadState('networkidle', { timeout: navigationTimeoutMs }).catch(() => {});
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
        for (let depth = 0; current && depth < 6; depth += 1) {
          const text = clean(current.innerText || current.textContent || '');
          if (text && !fallback) fallback = text;
          if (text.length >= 20 && text.length <= 140) return text;
          current = current.parentElement;
        }
        return fallback;
      };

      const candidates = Array.from(document.querySelectorAll('a[href], button, input, textarea, select, [role="button"], [role="link"]'))
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
          return {
            index,
            tag,
            role,
            type: el.getAttribute('type') || '',
            text,
            label,
            name,
            placeholder,
            href,
            disabled: Boolean(el.disabled || el.getAttribute('aria-disabled') === 'true'),
            nearbyText: nearbyText(el)
          };
        });

      return {
        url: window.location.href,
        title: document.title,
        candidates
      };
    });

    const totalCounts = new Map();
    for (const candidate of snapshot.candidates) {
      const key = `${candidate.tag}:${candidate.text || candidate.label}`;
      totalCounts.set(key, (totalCounts.get(key) || 0) + 1);
    }
    const occurrenceCounts = new Map();
    snapshot.candidates = snapshot.candidates.map((candidate) => {
      const key = `${candidate.tag}:${candidate.text || candidate.label}`;
      const occurrence = (occurrenceCounts.get(key) || 0) + 1;
      occurrenceCounts.set(key, occurrence);
      return {
        ...candidate,
        suggestedSelectors: suggestedSelectors(candidate, occurrence, totalCounts.get(key) || 0)
      };
    });

    fs.writeFileSync(outPath, JSON.stringify(snapshot, null, 2));
  } finally {
    await browser.close();
  }
}

function suggestedSelectors(candidate, occurrence, totalCount) {
  const selectors = [];

  if (candidate.tag === 'a' && candidate.href) {
    const token = lastPathToken(candidate.href);
    if (token) selectors.push(`a[href*="${attrEscape(token)}"]`);
  }
  if (candidate.tag === 'button' && candidate.text) {
    // nth-match is a last resort — the LLM should prefer constructing a :near() selector
    // from nearbyText rather than relying on position, but we keep it as a fallback option.
    if (totalCount > 1) {
      selectors.push(`:nth-match(button:has-text(${JSON.stringify(candidate.text)}), ${occurrence})`);
    }
    selectors.push(`button:has-text(${JSON.stringify(candidate.text)})`);
  }
  if (candidate.role && candidate.label) {
    selectors.push(`${candidate.tag || '*'}:has-text(${JSON.stringify(candidate.label)})`);
  }
  if (candidate.placeholder) {
    selectors.push(`[placeholder="${attrEscape(candidate.placeholder)}"]`);
  }
  if (candidate.name) {
    selectors.push(`[name="${attrEscape(candidate.name)}"]`);
  }
  return Array.from(new Set(selectors)).slice(0, 4);
}

function lastPathToken(href) {
  try {
    const url = new URL(href, 'http://localhost');
    return url.pathname.split('/').filter(Boolean).pop() || '';
  } catch {
    return '';
  }
}

function attrEscape(value) {
  return String(value || '').replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
