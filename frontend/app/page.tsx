'use client';

import { useState, useEffect } from 'react';

type Expectation = {
  urlContains?: string;
  textVisible?: string;
  elementVisible?: string;
};

type Step = {
  id: string;
  description?: string;
  action: string;
  selector?: string;
  target?: string;
  value?: string;
  allowRecovery?: boolean;
  expect?: Expectation;
};

type TestSpec = {
  name: string;
  baseUrl: string;
  steps: Step[];
};

type GenerateResponse = {
  spec?: TestSpec;
  validation: { valid: boolean; errors?: string[] };
  usedFallback: boolean;
};

type PromotionCandidate = {
  id: string;
  runId?: string;
  stepId: string;
  status: string;
  originalAction?: string;
  originalSelector?: string;
  proposedAction: string;
  proposedSelector: string;
  reasoning?: string;
  customerExplanation?: string;
  evidence?: string;
};

type StepResult = {
  stepId: string;
  action: string;
  mode?: string;
  status: string;
  durationMs: number;
  message?: string;
  unverifiedRecovery?: boolean;
  recovery?: {
    intent?: string;
    failedSelector?: string;
    recoveredSelector?: string;
    strategy?: string;
    reasoning?: string;
  };
  decisionTrace?: {
    mode?: string;
    attemptedAction?: string;
    attemptedSelector?: string;
    failure?: string;
    agentDecision?: string;
    selectedSelector?: string;
    reasoning?: string;
    customerExplanation?: string;
    evidence?: string;
  };
  promotionCandidate?: PromotionCandidate;
};

type RunResult = {
  runId: string;
  status: string;
  startedAt?: string;
  finishedAt?: string;
  steps: StepResult[];
  promotionCandidates?: PromotionCandidate[];
  artifacts?: Record<string, string>;
};

const statusColor: Record<string, string> = {
  passed: '#16a34a',
  recovered: '#d97706',
  failed: '#dc2626',
};

const modeColor: Record<string, string> = {
  deterministic: '#2563eb',
  agentic: '#7c3aed',
};

function StatusBadge({ status }: { status: string }) {
  return (
    <span style={{
      background: statusColor[status] || '#6b7280',
      color: '#fff',
      borderRadius: 4,
      padding: '2px 8px',
      fontSize: 12,
      fontWeight: 600,
      textTransform: 'uppercase',
      flexShrink: 0,
    }}>{status}</span>
  );
}

function ModeBadge({ mode }: { mode?: string }) {
  if (!mode) return null;
  return (
    <span style={{
      background: modeColor[mode] || '#6b7280',
      color: '#fff',
      borderRadius: 4,
      padding: '2px 8px',
      fontSize: 11,
      fontWeight: 500,
      flexShrink: 0,
    }}>{mode}</span>
  );
}

function RunNarrative({ result }: { result: RunResult }) {
  const total = result.steps.length;
  const passed = result.steps.filter((s) => s.status === 'passed').length;
  const recovered = result.steps.filter((s) => s.status === 'recovered').length;
  const failed = result.steps.filter((s) => s.status === 'failed').length;

  const durationMs = result.steps.reduce((sum, s) => sum + (s.durationMs || 0), 0);
  const durationSec = (durationMs / 1000).toFixed(1);

  let summary = `${durationSec}s · ${total} step${total !== 1 ? 's' : ''}: `;
  const parts: string[] = [];
  if (passed > 0) parts.push(`${passed} passed`);
  if (recovered > 0) parts.push(`${recovered} recovered by agent`);
  if (failed > 0) parts.push(`${failed} failed`);
  summary += parts.join(', ') + '.';

  if (recovered > 0) {
    const recoveredSteps = result.steps.filter((s) => s.status === 'recovered');
    const explanations = recoveredSteps
      .map((s) => s.decisionTrace?.customerExplanation || s.recovery?.reasoning)
      .filter(Boolean);
    if (explanations.length > 0) summary += ' ' + explanations.join(' ');
    const unverified = recoveredSteps.filter((s) => s.unverifiedRecovery);
    if (unverified.length > 0) {
      summary += ` Note: ${unverified.length} recovered step${unverified.length > 1 ? 's' : ''} lacked outcome validation.`;
    }
  }

  const bgColor = result.status === 'passed' ? '#f0fdf4' : result.status === 'recovered' ? '#fffbeb' : '#fef2f2';
  const borderColor = result.status === 'passed' ? '#bbf7d0' : result.status === 'recovered' ? '#fde68a' : '#fecaca';

  return (
    <div style={{ background: bgColor, border: `1px solid ${borderColor}`, borderRadius: 8, padding: 12, marginBottom: 12 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4 }}>
        <strong style={{ fontSize: 13 }}>Run {result.runId}</strong>
        <StatusBadge status={result.status} />
      </div>
      <p style={{ margin: 0, lineHeight: 1.5, fontSize: 13, color: '#374151' }}>{summary}</p>
    </div>
  );
}

export default function Page() {
  const [intent, setIntent] = useState('Open http://localhost:8181/ homepage, click on Add Book, click on Buy to complete purchase');
  const [simulateDrift, setSimulateDrift] = useState(true);
  const [spec, setSpec] = useState<TestSpec | null>(null);
  const [runResult, setRunResult] = useState<RunResult | null>(null);
  const [status, setStatus] = useState<{ text: string; isError: boolean } | null>(null);
  const [generating, setGenerating] = useState(false);
  const [running, setRunning] = useState(false);
  const [expandedSteps, setExpandedSteps] = useState<Set<string>>(new Set());
  const [promotionApproved, setPromotionApproved] = useState(false);

  const apiBase = process.env.NEXT_PUBLIC_API_BASE || 'http://localhost:8080';

  // Auto-expand failed and recovered steps; collapse passed ones.
  useEffect(() => {
    if (!runResult) return;
    setExpandedSteps(new Set(
      runResult.steps
        .filter((s) => s.status !== 'passed')
        .map((s) => s.stepId)
    ));
  }, [runResult?.runId]);


  async function generate() {
    setGenerating(true);
    setPromotionApproved(false);
    setStatus({ text: 'Generating test spec — inspecting page and calling LLM…', isError: false });
    try {
      const res = await fetch(`${apiBase}/v1/tests/generate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ intent, forceDriftFallback: simulateDrift }),
      });
      const data = await res.json();
      if (!res.ok) { setStatus({ text: `Generate failed: ${data?.error || `HTTP ${res.status}`}`, isError: true }); return; }
      const typed = data as GenerateResponse;
      if (!typed.validation?.valid) { setStatus({ text: `Validation failed: ${(typed.validation?.errors || []).join(', ')}`, isError: true }); return; }
      if (typed.spec) setSpec(typed.spec);
      setRunResult(null);
      const source = typed.usedFallback ? 'fallback mode' : 'LLM';
      setStatus({ text: `Spec ready · ${typed.spec?.steps.length} steps · ${source}`, isError: false });
    } catch (err) {
      setStatus({ text: `Generate request failed: ${err instanceof Error ? err.message : 'unknown error'}`, isError: true });
    } finally {
      setGenerating(false);
    }
  }

  async function runTest() {
    if (!spec) return;
    setRunning(true);
    setStatus({ text: 'Running Playwright execution…', isError: false });
    try {
      const res = await fetch(`${apiBase}/v1/tests/run`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ spec }),
      });
      const data = await res.json();
      if (!res.ok) { setStatus({ text: `Run failed: ${data?.error || `HTTP ${res.status}`}`, isError: true }); return; }
      setRunResult(data);
      setStatus(null);
    } catch (err) {
      setStatus({ text: `Run request failed: ${err instanceof Error ? err.message : 'unknown error'}`, isError: true });
    } finally {
      setRunning(false);
    }
  }

  async function reviewPromotion(candidate: PromotionCandidate, action: 'approve' | 'reject') {
    if (!runResult?.runId) return;
    setStatus({ text: `${action === 'approve' ? 'Approving' : 'Rejecting'} promotion…`, isError: false });
    try {
      const res = await fetch(`${apiBase}/v1/promotions/${candidate.id}/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ runId: runResult.runId }),
      });
      const data = await res.json();
      if (!res.ok) { setStatus({ text: `Promotion failed: ${data?.error || `HTTP ${res.status}`}`, isError: true }); return; }
      if (data.updatedSpec) {
        setSpec(data.updatedSpec);
        if (action === 'approve') setPromotionApproved(true);
      }
      if (data.runResult) setRunResult(data.runResult);
      setStatus(null);
    } catch (err) {
      setStatus({ text: `Promotion request failed: ${err instanceof Error ? err.message : 'unknown error'}`, isError: true });
    }
  }

  function toggleStep(stepId: string) {
    setExpandedSteps((prev) => {
      const next = new Set(prev);
      if (next.has(stepId)) next.delete(stepId); else next.add(stepId);
      return next;
    });
  }

  function scrollToStep(stepId: string) {
    document.getElementById(`trace-${stepId}`)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }

  function candidateForStep(step: StepResult): PromotionCandidate | undefined {
    if (step.promotionCandidate) return step.promotionCandidate;
    return runResult?.promotionCandidates?.find((c) => c.stepId === step.stepId);
  }

  return (
    <main style={{ maxWidth: 1240, margin: '40px auto', fontFamily: 'ui-sans-serif, system-ui', padding: '0 16px 40px' }}>

      {/* ── Header ── */}
      <h1 style={{ margin: '0 0 4px' }}>Hybrid Test Runner</h1>
      <p style={{ color: '#6b7280', margin: '0 0 16px', fontSize: 13 }}>
        Natural language → structured spec → Playwright execution → AI recovery → human review
      </p>

      {/* ── Controls ── */}
      <label htmlFor="intent" style={{ fontWeight: 500 }}>Test intent</label>
      <textarea
        id="intent"
        value={intent}
        onChange={(e) => setIntent(e.target.value)}
        rows={3}
        style={{ width: '100%', marginTop: 6, padding: 8, boxSizing: 'border-box', fontFamily: 'inherit' }}
      />

      <div style={{ display: 'flex', gap: 24, marginTop: 10, flexWrap: 'wrap', alignItems: 'center' }}>
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
          <input type="checkbox" checked={simulateDrift} onChange={(e) => setSimulateDrift(e.target.checked)} />
          <span>Simulate UI change</span>
          <span style={{ fontSize: 12, color: '#6b7280' }}>— breaks one selector to demonstrate automatic recovery</span>
        </label>
      </div>

      <div style={{ display: 'flex', gap: 12, marginTop: 14, alignItems: 'center' }}>
        <button
          onClick={generate}
          disabled={generating}
          style={{ padding: '8px 18px', cursor: generating ? 'default' : 'pointer', opacity: generating ? 0.6 : 1 }}
        >
          {generating ? 'Generating…' : 'Generate Test'}
        </button>
        <button
          onClick={runTest}
          disabled={!spec || running}
          style={{ padding: '8px 18px', cursor: (!spec || running) ? 'default' : 'pointer', opacity: (!spec || running) ? 0.6 : 1 }}
        >
          {running ? 'Running…' : 'Run Test'}
        </button>
      </div>

      {status && (
        <div style={{
          marginTop: 10,
          padding: '8px 14px',
          borderRadius: 6,
          fontSize: 13,
          background: status.isError ? '#fef2f2' : (generating || running) ? '#eff6ff' : '#f0fdf4',
          border: `1px solid ${status.isError ? '#fca5a5' : (generating || running) ? '#bfdbfe' : '#bbf7d0'}`,
          color: status.isError ? '#dc2626' : (generating || running) ? '#1d4ed8' : '#15803d',
          display: 'flex',
          alignItems: 'center',
          gap: 8,
        }}>
          {(generating || running) && !status.isError && (
            <span style={{
              display: 'inline-block', width: 12, height: 12,
              border: '2px solid currentColor', borderTopColor: 'transparent',
              borderRadius: '50%', animation: 'spin 0.8s linear infinite',
            }} />
          )}
          {status.text}
        </div>
      )}

      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>

      {/* ── Two-column body ── */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: runResult ? '1fr 1fr' : '1fr',
        gap: 28,
        marginTop: 24,
        alignItems: 'start',
      }}>

        {/* ── Left: Spec ── */}
        <div style={{ overflowY: 'auto', maxHeight: '78vh', paddingRight: 4 }}>
          <h2 style={{ marginTop: 0, marginBottom: 12 }}>Structured Spec</h2>

          {promotionApproved && (
            <div style={{
              background: '#f0fdf4', border: '1px solid #bbf7d0', borderRadius: 8,
              padding: '10px 14px', marginBottom: 12, display: 'flex',
              justifyContent: 'space-between', alignItems: 'flex-start', gap: 12,
            }}>
              <div>
                <strong style={{ fontSize: 13, color: '#15803d' }}>Spec updated from agent recovery</strong>
                <p style={{ margin: '2px 0 0', fontSize: 12, color: '#166534' }}>
                  The recovered selector has been promoted. The next run will execute this step deterministically — no agent needed.
                </p>
              </div>
              <button
                onClick={() => setPromotionApproved(false)}
                style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#6b7280', fontSize: 16, lineHeight: 1, flexShrink: 0 }}
              >✕</button>
            </div>
          )}

          {spec ? (
            <div style={{ display: 'grid', gap: 8 }}>
              {spec.steps.map((step) => (
                <div key={step.id} style={{
                  border: '1px solid #e5e7eb',
                  background: '#fff',
                  borderRadius: 6,
                  padding: 10,
                  fontSize: 13,
                }}>
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                    <code style={{ fontWeight: 600 }}>{step.id}</code>
                    <span style={{ background: '#f3f4f6', borderRadius: 4, padding: '1px 6px' }}>{step.action}</span>
                  </div>
                  {step.description && <p style={{ margin: '4px 0 0', color: '#374151' }}>{step.description}</p>}
                  {(step.selector || step.target) && (
                    <code style={{ display: 'block', marginTop: 4, color: '#6b7280', wordBreak: 'break-all' }}>
                      {step.selector || step.target}
                    </code>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <p style={{ color: '#6b7280' }}>No spec yet — enter a test intent and click Generate Test.</p>
          )}
        </div>

        {/* ── Right: Run results ── */}
        {runResult && (
          <div style={{ overflowY: 'auto', maxHeight: '78vh', paddingRight: 4 }}>
            <h2 style={{ marginTop: 0, marginBottom: 12 }}>Run Result</h2>
            <RunNarrative result={runResult} />

            {/* Step status strip */}
            <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap', marginBottom: 14 }}>
              {runResult.steps.map((step) => (
                <button
                  key={step.stepId}
                  onClick={() => scrollToStep(step.stepId)}
                  title={spec?.steps.find((s) => s.id === step.stepId)?.description || step.stepId}
                  style={{
                    background: statusColor[step.status] || '#6b7280',
                    color: '#fff',
                    border: 'none',
                    borderRadius: 4,
                    padding: '3px 9px',
                    fontSize: 11,
                    fontFamily: 'monospace',
                    cursor: 'pointer',
                    opacity: 0.9,
                  }}
                >
                  {step.stepId}
                </button>
              ))}
            </div>

            {/* Decision trace — collapsible cards */}
            <div style={{ display: 'grid', gap: 8 }}>
              {runResult.steps.map((step) => {
                const trace = step.decisionTrace || {};
                const candidate = candidateForStep(step);
                const mode = step.mode || trace.mode || 'deterministic';
                const specStep = spec?.steps.find((s) => s.id === step.stepId);
                const isExpanded = expandedSteps.has(step.stepId);
                const borderColor = step.status === 'failed' ? '#fca5a5' : step.status === 'recovered' ? '#fde68a' : '#e5e7eb';

                // Human-readable explanation for every step, not just recovered ones.
                const explanation =
                  trace.customerExplanation ||
                  step.message ||
                  (step.status === 'passed' ? 'Step ran as scripted.' : null);

                return (
                  <section
                    id={`trace-${step.stepId}`}
                    key={step.stepId}
                    style={{ border: `1px solid ${borderColor}`, borderRadius: 8, overflow: 'hidden' }}
                  >
                    {/* Clickable header */}
                    <div
                      onClick={() => toggleStep(step.stepId)}
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        gap: 12,
                        padding: '10px 14px',
                        cursor: 'pointer',
                        background: isExpanded ? '#fafafa' : '#fff',
                        userSelect: 'none',
                      }}
                    >
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                          <strong style={{ fontFamily: 'monospace', fontSize: 13 }}>{step.stepId}</strong>
                          <span style={{ color: '#6b7280', fontSize: 13 }}>{step.action}</span>
                          <ModeBadge mode={mode} />
                          {step.unverifiedRecovery && (
                            <span style={{ background: '#fee2e2', color: '#991b1b', borderRadius: 4, padding: '2px 7px', fontSize: 11 }}>
                              unverified
                            </span>
                          )}
                        </div>
                        {specStep?.description && (
                          <span style={{ fontSize: 12, color: '#6b7280', fontStyle: 'italic', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                            {specStep.description}
                          </span>
                        )}
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexShrink: 0 }}>
                        <StatusBadge status={step.status} />
                        <span style={{ color: '#9ca3af', fontSize: 14 }}>{isExpanded ? '▾' : '▸'}</span>
                      </div>
                    </div>

                    {/* Expanded detail */}
                    {isExpanded && (
                      <div style={{ padding: '0 14px 14px', borderTop: `1px solid ${borderColor}` }}>
                        <p style={{ margin: '10px 0', color: '#374151', lineHeight: 1.5, fontSize: 13 }}>
                          {explanation}
                        </p>

                        <dl style={{ display: 'grid', gridTemplateColumns: '150px 1fr', gap: '4px 8px', margin: 0, fontSize: 13 }}>
                          {trace.attemptedSelector && (
                            <>
                              <dt style={{ color: '#6b7280' }}>Attempted selector</dt>
                              <dd style={{ margin: 0, fontFamily: 'monospace', wordBreak: 'break-all' }}>{trace.attemptedSelector}</dd>
                            </>
                          )}
                          {trace.failure && (
                            <>
                              <dt style={{ color: '#dc2626' }}>Failure</dt>
                              <dd style={{ margin: 0, color: '#dc2626' }}>{trace.failure.split('\n')[0]}</dd>
                            </>
                          )}
                          {trace.agentDecision && (
                            <>
                              <dt style={{ color: '#6b7280' }}>Agent decision</dt>
                              <dd style={{ margin: 0 }}>{trace.agentDecision}</dd>
                            </>
                          )}
                          {trace.selectedSelector && (
                            <>
                              <dt style={{ color: '#6b7280' }}>Selected selector</dt>
                              <dd style={{ margin: 0, fontFamily: 'monospace', wordBreak: 'break-all' }}>{trace.selectedSelector}</dd>
                            </>
                          )}
                          {trace.reasoning && (
                            <>
                              <dt style={{ color: '#6b7280' }}>Reasoning</dt>
                              <dd style={{ margin: 0 }}>{trace.reasoning}</dd>
                            </>
                          )}
                          {trace.evidence && (
                            <>
                              <dt style={{ color: '#6b7280' }}>Evidence</dt>
                              <dd style={{ margin: 0 }}>{trace.evidence}</dd>
                            </>
                          )}
                          <dt style={{ color: '#6b7280' }}>Duration</dt>
                          <dd style={{ margin: 0 }}>{step.durationMs}ms</dd>
                        </dl>

                        {candidate && (
                          <div style={{ marginTop: 14, padding: 12, background: '#f9fafb', borderRadius: 6, border: '1px solid #e5e7eb' }}>
                            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                              <strong style={{ fontSize: 13 }}>Promotion candidate</strong>
                              <StatusBadge status={candidate.status} />
                            </div>
                            <p style={{ margin: '0 0 8px', color: '#374151', fontSize: 13 }}>{candidate.customerExplanation}</p>
                            <dl style={{ display: 'grid', gridTemplateColumns: '110px 1fr', gap: '4px 8px', margin: '0 0 10px', fontSize: 13 }}>
                              <dt style={{ color: '#6b7280' }}>Old selector</dt>
                              <dd style={{ margin: 0, fontFamily: 'monospace', wordBreak: 'break-all' }}>{candidate.originalSelector || '—'}</dd>
                              <dt style={{ color: '#6b7280' }}>New selector</dt>
                              <dd style={{ margin: 0, fontFamily: 'monospace', wordBreak: 'break-all' }}>{candidate.proposedSelector}</dd>
                              <dt style={{ color: '#6b7280' }}>Reasoning</dt>
                              <dd style={{ margin: 0 }}>{candidate.reasoning}</dd>
                            </dl>
                            <div style={{ display: 'flex', gap: 8 }}>
                              <button
                                onClick={() => reviewPromotion(candidate, 'approve')}
                                disabled={candidate.status !== 'pending'}
                                style={{ padding: '6px 14px', background: '#16a34a', color: '#fff', border: 'none', borderRadius: 4, cursor: candidate.status === 'pending' ? 'pointer' : 'default', opacity: candidate.status === 'pending' ? 1 : 0.5 }}
                              >
                                Approve
                              </button>
                              <button
                                onClick={() => reviewPromotion(candidate, 'reject')}
                                disabled={candidate.status !== 'pending'}
                                style={{ padding: '6px 14px', background: '#fff', border: '1px solid #d1d5db', borderRadius: 4, cursor: candidate.status === 'pending' ? 'pointer' : 'default', opacity: candidate.status === 'pending' ? 1 : 0.5 }}
                              >
                                Reject
                              </button>
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </section>
                );
              })}
            </div>
          </div>
        )}
      </div>

    </main>
  );
}
