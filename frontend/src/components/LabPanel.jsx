import { useEffect, useMemo, useState } from 'react';
import axios from 'axios';
import { CheckIcon, CircleIcon } from './Icons';

const API_URL = import.meta.env.VITE_API_URL || '';
const LOCAL_KEY = 'entropy_lab_progress_v1';

function readLocal() {
  try {
    return JSON.parse(localStorage.getItem(LOCAL_KEY) || '{}');
  } catch {
    return {};
  }
}

function writeLocal(progress) {
  try {
    localStorage.setItem(LOCAL_KEY, JSON.stringify(progress));
  } catch {
    /* storage unavailable */
  }
}

export function formatSeconds(total) {
  const s = Math.max(0, Math.floor(total || 0));
  const m = Math.floor(s / 60);
  const r = s % 60;
  return m > 0 ? `${m}m ${String(r).padStart(2, '0')}s` : `${r}s`;
}

function useElapsed(startedAt, running) {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    if (!running) return undefined;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [running]);
  if (!startedAt) return 0;
  return (now - new Date(startedAt).getTime()) / 1000;
}

const DIFFICULTY_ORDER = { Beginner: 0, Intermediate: 1, Advanced: 2 };

// Incidents are grouped: starters, single-machine incidents, then the
// production stack.
const GROUP_LABELS = ['Start here if you are new to Linux', 'Incidents', 'Production stack: proxy, app, database, live traffic'];
function groupOf(lab) {
  if (lab.starter) return 0;
  return lab.stack ? 2 : 1;
}

export default function LabPanel({ sessionId, authHeaders, lab, labVersion, onAction, onDismiss, onAskAI, onPostmortem, disabled }) {
  const [catalog, setCatalog] = useState([]);
  const [progress, setProgress] = useState(readLocal);
  const [tab, setTab] = useState('incident');
  const [pending, setPending] = useState(null);
  const [loadError, setLoadError] = useState('');

  useEffect(() => {
    if (!sessionId) return;
    axios
      .get(`${API_URL}/api/labs`, { headers: authHeaders(), params: { session_id: sessionId } })
      .then((res) => {
        setCatalog(res.data.labs || []);
        setLoadError('');
        const merged = { ...readLocal() };
        Object.entries(res.data.progress || {}).forEach(([id, c]) => {
          const prev = merged[id];
          if (!prev || c.hintsUsed < prev.hintsUsed || (c.hintsUsed === prev.hintsUsed && c.seconds < prev.seconds)) {
            merged[id] = c;
          }
        });
        writeLocal(merged);
        setProgress(merged);
      })
      .catch(() => setLoadError('Lab catalog unavailable.'));
  }, [sessionId, authHeaders, labVersion]);

  const grouped = useMemo(
    () =>
      catalog
        .filter((l) => l.kind === tab)
        .sort(
          (a, b) =>
            groupOf(a) - groupOf(b) || (DIFFICULTY_ORDER[a.difficulty] ?? 9) - (DIFFICULTY_ORDER[b.difficulty] ?? 9),
        ),
    [catalog, tab],
  );
  const incidentCount = catalog.filter((l) => l.kind === 'incident').length;
  const solvedCount = catalog.filter((l) => l.kind === 'incident' && progress[l.id]).length;

  if (lab) {
    return (
      <Mission lab={lab} onAction={onAction} onDismiss={onDismiss} onAskAI={onAskAI} onPostmortem={onPostmortem} disabled={disabled} />
    );
  }

  return (
    <section className="panel flex flex-col min-h-0">
      <header className="px-4 pt-3 pb-2 border-b border-line">
        <div className="flex items-baseline justify-between gap-2">
          <h2 className="text-sm font-semibold text-ink">Labs</h2>
          {incidentCount > 0 && (
            <span className="text-xs text-ink-muted tabular-nums">
              {solvedCount} of {incidentCount} incidents resolved
            </span>
          )}
        </div>
        {incidentCount > 0 && (
          <div className="mt-2 h-1 w-full bg-muted rounded-[2px] overflow-hidden" aria-hidden="true">
            <div
              className="h-full bg-accent transition-[width] duration-500"
              style={{ width: `${(solvedCount / incidentCount) * 100}%` }}
            />
          </div>
        )}
        <div className="mt-3 flex gap-1 text-xs" role="tablist">
          {[
            ['incident', 'Incidents'],
            ['demo', 'Demos'],
          ].map(([id, label]) => (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={tab === id}
              onClick={() => setTab(id)}
              className={`px-2.5 py-1 rounded-panel border ${
                tab === id ? 'border-accent text-accent bg-accent-soft' : 'border-transparent text-ink-muted hover:text-ink'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
      </header>

      <p className="px-4 pt-2 text-xs text-ink-muted">
        {tab === 'incident'
          ? 'Something is broken. Diagnose it in the terminal; objectives check themselves after every command.'
          : 'Short, observable failures. Watch the system view, then read the analysis.'}
      </p>

      {loadError && <p className="px-4 py-3 text-xs text-danger">{loadError}</p>}

      <ul className="px-2 py-2 overflow-y-auto min-h-0">
        {grouped.map((l, index) => {
          const done = progress[l.id];
          const firstOfGroup = index === 0 || groupOf(grouped[index - 1]) !== groupOf(l);
          return (
            <li key={l.id}>
              {firstOfGroup && tab === 'incident' && (
                <div className="px-2 pt-2 pb-1 text-[11px] font-medium text-ink-muted">
                  {GROUP_LABELS[groupOf(l)]}
                </div>
              )}
              <button
                type="button"
                disabled={disabled}
                onClick={() => setPending(l)}
                className="w-full text-left px-2 py-2.5 rounded-panel hover:bg-muted disabled:opacity-50 group"
              >
                <div className="flex items-start gap-2.5">
                  <span className={`mt-0.5 ${done ? 'text-success' : 'text-line'}`}>
                    {done ? <CheckIcon /> : <CircleIcon />}
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-baseline justify-between gap-2">
                      <span className="text-sm font-medium text-ink group-hover:text-accent">{l.title}</span>
                      <span className="text-[11px] text-ink-muted shrink-0 tabular-nums">{l.minutes} min</span>
                    </div>
                    <div className="text-xs text-ink-muted mt-0.5 line-clamp-2">{l.summary}</div>
                    <div className="mt-1 text-[11px] text-ink-muted">
                      {l.category} <span aria-hidden="true">/</span> {l.difficulty}
                      {done && (
                        <span className="text-success">
                          {' '}
                          <span aria-hidden="true">/</span> resolved in {formatSeconds(done.seconds)}
                          {done.hintsUsed > 0 ? `, ${done.hintsUsed} hint${done.hintsUsed === 1 ? '' : 's'}` : ', no hints'}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              </button>
            </li>
          );
        })}
      </ul>

      {pending && (
        <Briefing
          lab={pending}
          onCancel={() => setPending(null)}
          onStart={() => {
            onAction('start', pending.id);
            setPending(null);
          }}
        />
      )}
    </section>
  );
}

function Briefing({ lab, onCancel, onStart }) {
  useEffect(() => {
    const onKey = (e) => e.key === 'Escape' && onCancel();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onCancel]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[#14181f]/30 p-4" onClick={onCancel} role="presentation">
      <div
        className="panel max-w-lg w-full p-6 shadow-sm"
        role="dialog"
        aria-modal="true"
        aria-labelledby="briefing-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="text-xs text-ink-muted">
          {lab.kind === 'incident' ? 'Incident' : 'Demo'} <span aria-hidden="true">/</span> {lab.category}{' '}
          <span aria-hidden="true">/</span> {lab.difficulty} <span aria-hidden="true">/</span> about {lab.minutes} min
        </div>
        <h3 id="briefing-title" className="mt-1 text-lg font-semibold text-ink">
          {lab.title}
        </h3>
        <p className="mt-3 text-sm text-ink leading-relaxed">{lab.briefing}</p>

        {lab.objectives?.length > 0 && (
          <div className="mt-4">
            <div className="text-xs font-medium text-ink-muted mb-1.5">Objectives</div>
            <ul className="space-y-1">
              {lab.objectives.map((o) => (
                <li key={o.id} className="flex gap-2 text-sm text-ink">
                  <span className="text-line mt-0.5">
                    <CircleIcon />
                  </span>
                  {o.text}
                </li>
              ))}
            </ul>
          </div>
        )}

        <div className="mt-4">
          <div className="text-xs font-medium text-ink-muted mb-1.5">You will practice</div>
          <div className="flex flex-wrap gap-1.5">
            {lab.concepts.map((c) => (
              <span key={c} className="text-xs font-mono px-1.5 py-0.5 rounded-[4px] bg-muted text-ink">
                {c}
              </span>
            ))}
          </div>
        </div>

        {lab.kind === 'incident' && (
          <p className="mt-4 text-xs text-ink-muted">
            Starting resets any lab in progress. {lab.hintCount} hints are available; using fewer is recorded.
          </p>
        )}

        <div className="mt-6 flex gap-2 justify-end">
          <button type="button" onClick={onCancel} className="text-sm px-4 py-2 rounded-panel border border-line hover:bg-muted">
            Cancel
          </button>
          <button
            type="button"
            onClick={onStart}
            autoFocus
            className="text-sm px-4 py-2 rounded-panel bg-accent text-white hover:opacity-90"
          >
            {lab.kind === 'incident' ? 'Start incident' : 'Run demo'}
          </button>
        </div>
      </div>
    </div>
  );
}

function Mission({ lab, onAction, onDismiss, onAskAI, onPostmortem, disabled }) {
  const running = lab.state === 'preparing' || lab.state === 'active';
  const elapsed = useElapsed(lab.startedAt, running);
  const shownElapsed = running ? elapsed : lab.elapsedSeconds;
  const [showBriefing, setShowBriefing] = useState(true);
  const isIncident = lab.kind === 'incident';
  const total = lab.objectives?.length || 0;

  const stateLabel = {
    preparing: 'Preparing',
    active: isIncident ? 'In progress' : 'Running',
    completed: isIncident ? 'Resolved' : 'Finished',
    abandoned: 'Abandoned',
    failed: 'Setup failed',
  }[lab.state];
  const stateColor = {
    preparing: 'text-warning',
    active: 'text-accent',
    completed: 'text-success',
    abandoned: 'text-ink-muted',
    failed: 'text-danger',
  }[lab.state];

  return (
    <section className="panel flex flex-col min-h-0">
      <header className="px-4 pt-3 pb-3 border-b border-line">
        <div className="flex items-center justify-between gap-2 text-xs">
          <span className="text-ink-muted">
            {isIncident ? 'Incident' : 'Demo'} <span aria-hidden="true">/</span> {lab.category}{' '}
            <span aria-hidden="true">/</span> {lab.difficulty}
          </span>
          <span className={`font-medium ${stateColor}`}>{stateLabel}</span>
        </div>
        <h2 className="mt-1 text-base font-semibold text-ink leading-snug">{lab.title}</h2>
        <div className="mt-1 flex gap-4 text-xs text-ink-muted tabular-nums">
          <span>{formatSeconds(shownElapsed)}</span>
          {isIncident && (
            <span>
              {lab.objectivesMet} of {total} objectives
            </span>
          )}
          {isIncident && <span>{lab.hintsUsed} hints used</span>}
        </div>
      </header>

      <div className="px-4 py-3 overflow-y-auto min-h-0 space-y-4">
        {lab.message && <p className="text-xs text-ink-muted">{lab.message}</p>}

        <div>
          <button
            type="button"
            onClick={() => setShowBriefing((v) => !v)}
            className="text-xs font-medium text-ink-muted hover:text-ink"
            aria-expanded={showBriefing}
          >
            Briefing {showBriefing ? '(hide)' : '(show)'}
          </button>
          {showBriefing && <p className="mt-1 text-sm text-ink leading-relaxed">{lab.briefing}</p>}
        </div>

        {isIncident && lab.alerts?.length > 0 && running && (
          <div className="rounded-panel border border-line bg-muted px-3 py-2.5" role="status">
            <div className="text-xs font-medium text-warning">Something you fixed broke again</div>
            <ul className="mt-1 space-y-1">
              {lab.alerts.map((a, i) => (
                <li key={`${a.objectiveId}-${i}`} className="text-sm text-ink leading-relaxed">
                  {a.message}
                </li>
              ))}
            </ul>
          </div>
        )}

        {isIncident && (
          <div>
            <div className="text-xs font-medium text-ink-muted mb-1.5">Objectives</div>
            <ul className="space-y-1.5">
              {lab.objectives.map((o) => (
                <li key={o.id} className="flex gap-2 text-sm">
                  <span className={`mt-0.5 transition-colors ${o.done ? 'text-success' : 'text-line'}`}>
                    {o.done ? <CheckIcon /> : <CircleIcon />}
                  </span>
                  <span className={o.done ? 'text-ink-muted line-through decoration-line' : 'text-ink'}>{o.text}</span>
                </li>
              ))}
            </ul>
          </div>
        )}

        {isIncident && lab.hints?.length > 0 && (
          <div>
            <div className="text-xs font-medium text-ink-muted mb-1.5">Hints</div>
            <ol className="space-y-2">
              {lab.hints.map((h, i) => (
                <li key={h} className="text-sm text-ink bg-muted rounded-panel px-3 py-2 leading-relaxed">
                  <span className="text-xs text-ink-muted mr-1.5 tabular-nums">{i + 1}.</span>
                  <InlineCode text={h} />
                </li>
              ))}
            </ol>
          </div>
        )}

        {lab.state === 'completed' && isIncident && (
          <div className="rounded-panel border border-line bg-accent-soft px-3 py-3">
            <div className="text-sm font-semibold text-success">
              Resolved in {formatSeconds(lab.elapsedSeconds)} with {lab.hintsUsed} hint{lab.hintsUsed === 1 ? '' : 's'}
            </div>
            {lab.debrief && <p className="mt-2 text-sm text-ink leading-relaxed">{lab.debrief}</p>}
          </div>
        )}
        {lab.state === 'completed' && !isIncident && (
          <p className="text-sm text-ink">The analysis is in the explanation panel below.</p>
        )}
      </div>

      <footer className="px-4 py-3 border-t border-line flex flex-wrap gap-2">
        {running && isIncident && (
          <>
            <button
              type="button"
              disabled={disabled || lab.state !== 'active'}
              onClick={() => onAction('check')}
              className="text-xs font-medium px-3 py-1.5 rounded-panel bg-accent text-white hover:opacity-90 disabled:opacity-50"
            >
              Check objectives
            </button>
            <button
              type="button"
              disabled={disabled || lab.state !== 'active' || lab.hintsUsed >= lab.hintCount}
              onClick={() => onAction('hint')}
              className="text-xs font-medium px-3 py-1.5 rounded-panel border border-line text-ink hover:bg-muted disabled:opacity-50"
            >
              {lab.hintsUsed >= lab.hintCount ? 'No more hints' : `Reveal hint ${lab.hintsUsed + 1} of ${lab.hintCount}`}
            </button>
            <button
              type="button"
              disabled={disabled || lab.state !== 'active'}
              onClick={() => onAskAI('I am stuck on this lab. What should I look at next, without giving away the fix?')}
              className="text-xs font-medium px-3 py-1.5 rounded-panel border border-line text-ink hover:bg-muted disabled:opacity-50"
            >
              Ask for a nudge
            </button>
            <button
              type="button"
              disabled={disabled}
              onClick={() => onAction('abandon')}
              className="text-xs font-medium px-3 py-1.5 text-ink-muted hover:text-danger ml-auto"
            >
              Abandon
            </button>
          </>
        )}
        {running && !isIncident && (
          <button
            type="button"
            disabled={disabled}
            onClick={() => onAction('abandon')}
            className="text-xs font-medium px-3 py-1.5 text-ink-muted hover:text-danger"
          >
            Stop demo
          </button>
        )}
        {!running && isIncident && lab.state !== 'failed' && onPostmortem && (
          <button
            type="button"
            onClick={() => onPostmortem(lab.id)}
            className="text-xs font-medium px-3 py-1.5 rounded-panel bg-accent text-white hover:opacity-90"
          >
            Write postmortem
          </button>
        )}
        {!running && (
          <button
            type="button"
            onClick={onDismiss}
            className="text-xs font-medium px-3 py-1.5 rounded-panel border border-line text-ink hover:bg-muted"
          >
            Back to labs
          </button>
        )}
      </footer>
    </section>
  );
}

// InlineCode renders `backtick` spans in hints as code.
function InlineCode({ text }) {
  const parts = text.split(/(`[^`]+`)/g);
  return parts.map((p, i) =>
    p.startsWith('`') && p.endsWith('`') ? (
      <code key={i} className="font-mono text-[12px] bg-surface border border-line rounded-[4px] px-1 py-px">
        {p.slice(1, -1)}
      </code>
    ) : (
      <span key={i}>{p}</span>
    ),
  );
}
