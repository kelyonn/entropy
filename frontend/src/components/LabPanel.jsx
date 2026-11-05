import { useEffect, useMemo, useRef, useState } from 'react';
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
const GROUP_LABELS = [
  'Start here if you are new to Linux',
  'Incidents',
  'Production stack: proxy, app, database, live traffic',
  'Community and generated (each one validated in a sandbox)',
];
function groupOf(lab) {
  if (lab.starter) return 0;
  if (lab.community) return 3;
  return lab.stack ? 2 : 1;
}

export default function LabPanel({
  sessionId,
  authHeaders,
  lab,
  labVersion,
  onAction,
  onDismiss,
  onAskAI,
  onPostmortem,
  gameDay,
  onDismissGameDay,
  disabled,
}) {
  const [confirmGameDay, setConfirmGameDay] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [reload, setReload] = useState(0);
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
          const better =
            !prev ||
            (c.score ?? 0) > (prev.score ?? 0) ||
            ((c.score ?? 0) === (prev.score ?? 0) &&
              (c.hintsUsed < prev.hintsUsed || (c.hintsUsed === prev.hintsUsed && c.seconds < prev.seconds)));
          if (better) {
            merged[id] = c;
          }
        });
        writeLocal(merged);
        setProgress(merged);
      })
      .catch(() => setLoadError('Lab catalog unavailable.'));
  }, [sessionId, authHeaders, labVersion, reload]);

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

  const banner = gameDay && <GameDayBanner gameDay={gameDay} onDismiss={onDismissGameDay} />;

  if (lab) {
    return (
      <div className="flex flex-col gap-3 min-h-0">
        {banner}
        <Mission lab={lab} onAction={onAction} onDismiss={onDismiss} onAskAI={onAskAI} onPostmortem={onPostmortem} disabled={disabled} />
      </div>
    );
  }
  if (gameDay && gameDay.state === 'running') {
    return banner;
  }

  return (
    <section className="panel flex flex-col min-h-0">
      <header className="px-4 pt-3 pb-2 border-b border-line">
        <div className="flex items-baseline justify-between gap-2">
          <div className="flex items-baseline gap-3">
            <h2 className="text-sm font-semibold text-ink">Labs</h2>
            <button
              type="button"
              disabled={disabled}
              onClick={() => setConfirmGameDay(true)}
              className="text-xs font-medium text-danger hover:underline disabled:opacity-50"
            >
              Start game day
            </button>
          </div>
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
            ['skills', 'Skills'],
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

      {gameDay && <div className="px-4 pt-3">{banner}</div>}

      <p className="px-4 pt-2 text-xs text-ink-muted">
        {tab === 'incident'
          ? 'Something is broken. Diagnose it in the terminal; objectives check themselves after every command.'
          : tab === 'demo'
            ? 'Short, observable failures. Watch the system view, then read the analysis.'
            : 'Your progress by area. Interview grades count separately from practice runs.'}
      </p>

      {tab === 'skills' && <Skills catalog={catalog} progress={progress} />}

      {loadError && <p className="px-4 py-3 text-xs text-danger">{loadError}</p>}

      <ul className={`px-2 py-2 overflow-y-auto min-h-0 ${tab === 'skills' ? 'hidden' : ''}`}>
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
                    {l.generated && <div className="text-[11px] text-accent mt-0.5">AI generated, sandbox validated</div>}
                    <div className="mt-1 text-[11px] text-ink-muted">
                      {l.category} <span aria-hidden="true">/</span> {l.difficulty}
                      {progress[`${l.id}:interview`] && (
                        <span className="text-ink">
                          {' '}
                          <span aria-hidden="true">/</span> interview {progress[`${l.id}:interview`].grade}
                        </span>
                      )}
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

      {tab === 'incident' && (
        <div className="px-4 py-2 border-t border-line">
          <button
            type="button"
            disabled={disabled}
            onClick={() => setGenerating(true)}
            className="text-xs font-medium text-accent hover:underline disabled:opacity-50"
          >
            Generate a new lab with AI
          </button>
        </div>
      )}

      {generating && (
        <GenerateLab
          sessionId={sessionId}
          authHeaders={authHeaders}
          onClose={() => setGenerating(false)}
          onPublished={() => setReload((r) => r + 1)}
        />
      )}

      {pending && (
        <Briefing
          lab={pending}
          onCancel={() => setPending(null)}
          onStart={(mode) => {
            onAction('start', pending.id, mode);
            setPending(null);
          }}
        />
      )}

      {confirmGameDay && (
        <ConfirmGameDay
          best={progress.gameday}
          onCancel={() => setConfirmGameDay(false)}
          onStart={() => {
            onAction('gameday');
            setConfirmGameDay(false);
          }}
        />
      )}
    </section>
  );
}

function Skills({ catalog, progress }) {
  const areas = {};
  catalog
    .filter((l) => l.kind === 'incident')
    .forEach((l) => {
      const a = (areas[l.category] = areas[l.category] || { total: 0, done: 0, interviews: [] });
      a.total += 1;
      if (progress[l.id]) a.done += 1;
      const iv = progress[`${l.id}:interview`];
      if (iv?.grade) a.interviews.push(iv.grade);
    });
  const order = ['Start here', 'Processes', 'Storage', 'Memory', 'Permissions', 'Networking', 'Production'];
  const names = Object.keys(areas).sort((a, b) => (order.indexOf(a) + 99) % 99 - (order.indexOf(b) + 99) % 99);
  const level = (a) => {
    const pct = a.done / a.total;
    if (pct === 1 && a.interviews.length === a.total && a.interviews.every((g) => g === 'A' || g === 'B')) return 'Expert';
    if (pct === 1) return 'Proficient';
    if (pct >= 0.5) return 'Practicing';
    if (pct > 0) return 'Started';
    return 'Not started';
  };
  const gd = progress.gameday;
  return (
    <div className="px-4 py-3 overflow-y-auto min-h-0 space-y-3">
      {names.map((n) => {
        const a = areas[n];
        return (
          <div key={n}>
            <div className="flex items-baseline justify-between text-xs">
              <span className="font-medium text-ink">{n}</span>
              <span className="text-ink-muted tabular-nums">
                {level(a)}
                {a.interviews.length > 0 ? `, interview ${a.interviews.join(' ')}` : ''} <span aria-hidden="true">/</span>{' '}
                {a.done} of {a.total}
              </span>
            </div>
            <div className="mt-1 h-1.5 w-full bg-muted rounded-[2px] overflow-hidden" aria-hidden="true">
              <div className="h-full bg-accent transition-[width] duration-500" style={{ width: `${(a.done / a.total) * 100}%` }} />
            </div>
          </div>
        );
      })}
      <p className="text-xs text-ink-muted pt-1">
        {gd ? `Best game day: ${gd.score}% error budget left in ${formatSeconds(gd.seconds)}.` : 'No game day completed yet.'}
        {' '}Expert means every lab in the area resolved and interviewed at B or better.
      </p>
    </div>
  );
}

function ConfirmGameDay({ best, onCancel, onStart }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[#14181f]/30 p-4" onClick={onCancel} role="presentation">
      <div className="panel max-w-md w-full p-6" role="dialog" aria-modal="true" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-semibold text-ink">Game day</h3>
        <p className="mt-2 text-sm text-ink leading-relaxed">
          Three random incidents, back to back, sharing one error budget. The budget burns while anything is broken and
          pauses between pages. Hints still work, but every second counts. Run out of budget and the game is over.
        </p>
        {best && (
          <p className="mt-2 text-xs text-ink-muted">
            Your best: {best.score}% budget left in {formatSeconds(best.seconds)}.
          </p>
        )}
        <div className="mt-6 flex gap-2 justify-end">
          <button type="button" onClick={onCancel} className="text-sm px-4 py-2 rounded-panel border border-line hover:bg-muted">
            Cancel
          </button>
          <button type="button" onClick={onStart} autoFocus className="text-sm px-4 py-2 rounded-panel bg-danger text-white hover:opacity-90">
            Start game day
          </button>
        </div>
      </div>
    </div>
  );
}

function GameDayBanner({ gameDay, onDismiss }) {
  const { state, index, total, resolved, budget } = gameDay;
  const color = budget > 50 ? 'var(--accent)' : budget > 20 ? 'var(--warning)' : 'var(--danger)';
  return (
    <section className="panel px-4 py-3" aria-live="polite">
      <div className="flex items-baseline justify-between text-xs">
        <span className="font-semibold text-danger">
          {state === 'running' ? `Game day: incident ${Math.min(index + 1, total)} of ${total}` : state === 'won' ? 'Game day complete' : 'Game day over'}
        </span>
        <span className="tabular-nums text-ink-muted">{formatSeconds(gameDay.elapsedSeconds)}</span>
      </div>
      <div className="mt-2 flex items-center gap-2 text-xs">
        <span className="text-ink-muted w-20 shrink-0">Error budget</span>
        <div className="flex-1 h-2 bg-muted rounded-[2px] overflow-hidden" aria-hidden="true">
          <div className="h-full transition-[width] duration-700" style={{ width: `${Math.max(0, budget)}%`, background: color }} />
        </div>
        <span className="tabular-nums w-10 text-right" style={{ color }}>
          {budget >= 99.95 ? '100' : budget.toFixed(1)}%
        </span>
      </div>
      {state !== 'running' && (
        <div className="mt-2 flex items-center justify-between text-sm">
          <span className="text-ink">
            {resolved} of {total} resolved{state === 'won' ? `, ${Math.round(budget)}% budget left.` : '.'}
          </span>
          <button type="button" onClick={onDismiss} className="text-xs font-medium text-accent">
            Dismiss
          </button>
        </div>
      )}
    </section>
  );
}

function Briefing({ lab, onCancel, onStart }) {
  const [mode, setMode] = useState('practice');
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
          <fieldset className="mt-5">
            <legend className="text-xs font-medium text-ink-muted mb-1.5">Mode</legend>
            <div className="grid grid-cols-2 gap-2">
              {[
                ['practice', 'Practice', `${lab.hintCount} hints and AI help available.`],
                ['interview', 'Interview', `No hints or AI. ${lab.minutes} minute limit. Graded A to F.`],
              ].map(([id, label, desc]) => (
                <label
                  key={id}
                  className={`cursor-pointer rounded-panel border px-3 py-2 ${
                    mode === id ? 'border-accent bg-accent-soft' : 'border-line hover:bg-muted'
                  }`}
                >
                  <input type="radio" name="mode" value={id} checked={mode === id} onChange={() => setMode(id)} className="sr-only" />
                  <div className="text-sm font-medium text-ink">{label}</div>
                  <div className="text-xs text-ink-muted mt-0.5">{desc}</div>
                </label>
              ))}
            </div>
            <p className="mt-2 text-xs text-ink-muted">Starting resets any lab in progress.</p>
          </fieldset>
        )}

        <div className="mt-6 flex gap-2 justify-end">
          <button type="button" onClick={onCancel} className="text-sm px-4 py-2 rounded-panel border border-line hover:bg-muted">
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onStart(lab.kind === 'incident' ? mode : 'practice')}
            autoFocus
            className="text-sm px-4 py-2 rounded-panel bg-accent text-white hover:opacity-90"
          >
            {lab.kind !== 'incident' ? 'Run demo' : mode === 'interview' ? 'Start interview' : 'Start incident'}
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
  const interview = lab.mode === 'interview';
  const remaining = interview && lab.timeLimitSeconds ? Math.max(0, lab.timeLimitSeconds - shownElapsed) : null;

  const stateLabel = {
    preparing: 'Preparing',
    active: isIncident ? 'In progress' : 'Running',
    completed: isIncident ? 'Resolved' : 'Finished',
    abandoned: 'Abandoned',
    failed: interview && lab.grade === 'F' ? 'Time expired' : 'Setup failed',
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
            {interview ? 'Interview' : lab.mode === 'gameday' ? 'Game day' : isIncident ? 'Incident' : 'Demo'}{' '}
            <span aria-hidden="true">/</span> {lab.category}{' '}
            <span aria-hidden="true">/</span> {lab.difficulty}
          </span>
          <span className={`font-medium ${stateColor}`}>{stateLabel}</span>
        </div>
        <h2 className="mt-1 text-base font-semibold text-ink leading-snug">{lab.title}</h2>
        <div className="mt-1 flex gap-4 text-xs text-ink-muted tabular-nums">
          {remaining != null && running ? (
            <span className={remaining < 60 ? 'text-danger font-medium' : 'text-ink'}>{formatSeconds(remaining)} left</span>
          ) : (
            <span>{formatSeconds(shownElapsed)}</span>
          )}
          {isIncident && (
            <span>
              {lab.objectivesMet} of {total} objectives
            </span>
          )}
          {isIncident && !interview && <span>{lab.hintsUsed} hints used</span>}
          {interview && running && <span>hints and AI off</span>}
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

        {lab.grade && (
          <div className="rounded-panel border border-line px-3 py-3 flex items-center gap-4">
            <div className={`text-4xl font-bold tabular-nums ${lab.grade <= 'B' ? 'text-success' : lab.grade <= 'D' ? 'text-warning' : 'text-danger'}`}>
              {lab.grade}
            </div>
            <div className="text-sm text-ink">
              <div className="font-semibold">Interview score {lab.score}/100</div>
              <div className="text-xs text-ink-muted mt-0.5">
                Scored on time against par, failed commands, and fixes you undid. Written feedback is in the explanation panel.
              </div>
            </div>
          </div>
        )}

        {lab.state === 'completed' && isIncident && (
          <div className="rounded-panel border border-line bg-accent-soft px-3 py-3">
            <div className="text-sm font-semibold text-success">
              Resolved in {formatSeconds(lab.elapsedSeconds)}
              {interview ? '' : ` with ${lab.hintsUsed} hint${lab.hintsUsed === 1 ? '' : 's'}`}
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
            {!interview && (
              <>
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
              </>
            )}
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

function GenerateLab({ sessionId, authHeaders, onClose, onPublished }) {
  const [topic, setTopic] = useState('');
  const [difficulty, setDifficulty] = useState('Intermediate');
  const [job, setJob] = useState(null);
  const [error, setError] = useState('');
  // The parent re-renders every second; keep callbacks in a ref so the poll
  // timer is not reset on every render.
  const cb = useRef({ authHeaders, onPublished });
  cb.current = { authHeaders, onPublished };

  useEffect(() => {
    if (!job || job.state === 'published' || job.state === 'failed') return undefined;
    const t = setTimeout(async () => {
      try {
        const res = await axios.get(`${API_URL}/api/labs/generate`, {
          headers: cb.current.authHeaders(),
          params: { job: job.id },
        });
        setJob(res.data);
        if (res.data.state === 'published') cb.current.onPublished();
      } catch {
        setError('Lost track of the generation job.');
      }
    }, 2000);
    return () => clearTimeout(t);
  }, [job]);

  const start = async (e) => {
    e.preventDefault();
    setError('');
    try {
      const res = await axios.post(
        `${API_URL}/api/labs/generate`,
        { topic, difficulty },
        { headers: authHeaders(), params: { session_id: sessionId } },
      );
      setJob(res.data);
    } catch (err) {
      setError(err.response?.data || err.message);
    }
  };

  const busy = job && (job.state === 'generating' || job.state === 'validating');
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[#14181f]/30 p-4" onClick={busy ? undefined : onClose} role="presentation">
      <div className="panel max-w-lg w-full p-6" role="dialog" aria-modal="true" aria-label="Generate a lab" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-semibold text-ink">Generate a lab</h3>
        <p className="mt-1 text-sm text-ink-muted">
          Describe an incident. The AI writes the setup, checks, hints, and a reference fix. Entropy then runs it in a
          throwaway sandbox and publishes it only if setup breaks the system and the fix repairs it.
        </p>
        {!job && (
          <form onSubmit={start} className="mt-4 space-y-3">
            <textarea
              value={topic}
              onChange={(e) => setTopic(e.target.value)}
              rows={3}
              maxLength={200}
              placeholder="For example: a cron-style job that keeps filling the disk, or a service stuck because its lock file was left behind"
              className="w-full text-sm px-3 py-2 rounded-panel border border-line bg-surface text-ink placeholder:text-ink-muted focus:outline-none focus:border-accent"
            />
            <div className="flex items-center justify-between gap-2">
              <select
                value={difficulty}
                onChange={(e) => setDifficulty(e.target.value)}
                className="text-sm px-2 py-1.5 rounded-panel border border-line bg-surface text-ink"
                aria-label="Difficulty"
              >
                <option>Beginner</option>
                <option>Intermediate</option>
                <option>Advanced</option>
              </select>
              <div className="flex gap-2">
                <button type="button" onClick={onClose} className="text-sm px-4 py-2 rounded-panel border border-line hover:bg-muted">
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={topic.trim().length < 4}
                  className="text-sm px-4 py-2 rounded-panel bg-accent text-white disabled:opacity-50"
                >
                  Generate
                </button>
              </div>
            </div>
          </form>
        )}
        {job && (
          <div className="mt-4 space-y-2 text-sm" aria-live="polite">
            <ol className="space-y-1.5">
              {[
                ['generating', `Writing the lab${job.attempt > 1 ? ` (attempt ${job.attempt})` : ''}`],
                ['validating', 'Validating in a throwaway sandbox'],
                ['published', 'Published'],
              ].map(([state, label]) => {
                const order = ['generating', 'validating', 'published'];
                const cur = job.state === 'failed' ? -1 : order.indexOf(job.state);
                const idx = order.indexOf(state);
                const done = cur > idx || job.state === 'published';
                const active = cur === idx && job.state !== 'published';
                return (
                  <li key={state} className="flex items-center gap-2">
                    <span className={done ? 'text-success' : 'text-line'}>{done ? <CheckIcon /> : <CircleIcon />}</span>
                    <span className={active ? 'text-ink font-medium' : done ? 'text-ink' : 'text-ink-muted'}>{label}</span>
                    {active && <span className="spinner" aria-hidden="true" />}
                  </li>
                );
              })}
            </ol>
            {job.title && <p className="text-ink-muted">{job.title}</p>}
            {job.state === 'published' && <p className="text-success">It is in the Community and generated group.</p>}
            {job.state === 'failed' && <p className="text-danger">{job.message}</p>}
            {job.problems?.length > 0 && (
              <ul className="text-xs text-ink-muted list-disc pl-5">
                {job.problems.slice(0, 5).map((p) => (
                  <li key={p}>{p}</li>
                ))}
              </ul>
            )}
            {!busy && (
              <div className="pt-2 flex justify-end">
                <button type="button" onClick={onClose} className="text-sm px-4 py-2 rounded-panel border border-line hover:bg-muted">
                  Close
                </button>
              </div>
            )}
          </div>
        )}
        {error && <p className="mt-3 text-sm text-danger">{String(error)}</p>}
      </div>
    </div>
  );
}
