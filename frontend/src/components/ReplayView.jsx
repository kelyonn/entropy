import { useEffect, useMemo, useRef, useState } from 'react';
import axios from 'axios';

const API_URL = import.meta.env.VITE_API_URL || '';

const KIND_STYLE = {
  lab: ['lab', 'text-accent'],
  objective: ['goal met', 'text-success'],
  regression: ['regressed', 'text-warning'],
  hint: ['hint', 'text-ink-muted'],
  crash: ['crash', 'text-danger'],
  ai: ['explained', 'text-ink-muted'],
};

function at(ev) {
  return new Date(ev.at).getTime();
}

// ReplayView plays back a recorded session: commands and their output in a
// terminal well, with lab milestones beside them. Gaps longer than three
// seconds are compressed so a ten-minute incident watches in about a minute.
export default function ReplayView({ token, labId }) {
  const [events, setEvents] = useState(null);
  const [error, setError] = useState('');
  const [index, setIndex] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(2);
  const timer = useRef(0);
  const termRef = useRef(null);

  useEffect(() => {
    axios
      .get(`${API_URL}/api/replay`, { params: { token, lab: labId || undefined }, headers: { 'X-Guest': '1' } })
      .then((res) => {
        const evs = (res.data.events || []).filter((e) => e.kind !== 'ai' || e.message);
        setEvents(evs);
        setIndex(evs.length ? 1 : 0);
      })
      .catch((err) => setError(err.response?.status === 404 ? 'This replay link has expired or does not exist.' : 'Replay unavailable.'));
  }, [token, labId]);

  useEffect(() => {
    clearTimeout(timer.current);
    if (!playing || !events || index >= events.length) {
      if (events && index >= events.length) setPlaying(false);
      return undefined;
    }
    const gap = index > 0 ? at(events[index]) - at(events[index - 1]) : 0;
    const delay = Math.min(Math.max(gap, 300), 3000) / speed;
    timer.current = setTimeout(() => setIndex((i) => i + 1), delay);
    return () => clearTimeout(timer.current);
  }, [playing, index, events, speed]);

  useEffect(() => {
    if (termRef.current) termRef.current.scrollTop = termRef.current.scrollHeight;
  }, [index]);

  const shown = useMemo(() => (events ? events.slice(0, index) : []), [events, index]);
  const title = useMemo(() => {
    const start = events?.find((e) => e.kind === 'lab' && e.message.startsWith('started'));
    return start ? start.message.replace(/^started: /, '') : 'Sandbox session';
  }, [events]);
  const start = events?.length ? at(events[0]) : 0;
  const elapsed = shown.length ? Math.round((at(shown[shown.length - 1]) - start) / 1000) : 0;

  return (
    <div className="min-h-screen bg-canvas text-ink">
      <header className="bg-surface border-b border-line">
        <div className="max-w-6xl mx-auto px-4 md:px-6 py-3 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-baseline gap-3">
            <a href="/" className="text-xl font-bold tracking-tight text-accent">
              ENTROPY
            </a>
            <span className="text-sm text-ink-muted">replay</span>
          </div>
          <a href="/" className="text-xs font-medium px-3 py-1.5 rounded-panel bg-accent text-white">
            Start your own sandbox
          </a>
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-4 md:px-6 py-5">
        {error && <div className="panel px-4 py-3 text-sm text-danger">{error}</div>}
        {!error && !events && <div className="text-sm text-ink-muted">Loading replay...</div>}
        {events && (
          <>
            <div className="flex flex-wrap items-end justify-between gap-3 mb-3">
              <div>
                <h1 className="text-lg font-semibold">{title}</h1>
                <p className="text-xs text-ink-muted">
                  {events.filter((e) => e.kind === 'command').length} commands, recorded{' '}
                  {events.length ? new Date(events[0].at).toLocaleString() : ''}
                </p>
              </div>
              <div className="flex items-center gap-2 text-xs">
                <button
                  type="button"
                  onClick={() => {
                    if (index >= events.length) setIndex(1);
                    setPlaying((p) => !p);
                  }}
                  className="font-medium px-3 py-1.5 rounded-panel bg-accent text-white"
                >
                  {playing ? 'Pause' : index >= events.length ? 'Replay' : 'Play'}
                </button>
                {[1, 2, 4].map((s) => (
                  <button
                    key={s}
                    type="button"
                    onClick={() => setSpeed(s)}
                    className={`px-2 py-1.5 rounded-panel border ${speed === s ? 'border-accent text-accent' : 'border-line text-ink-muted'}`}
                  >
                    {s}x
                  </button>
                ))}
                <span className="tabular-nums text-ink-muted w-14 text-right">
                  {Math.floor(elapsed / 60)}:{String(elapsed % 60).padStart(2, '0')}
                </span>
              </div>
            </div>
            <input
              type="range"
              min={0}
              max={events.length}
              value={index}
              onChange={(e) => {
                setPlaying(false);
                setIndex(Number(e.target.value));
              }}
              className="w-full accent-[var(--accent)] mb-4"
              aria-label="Replay position"
            />
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
              <div ref={termRef} className="lg:col-span-2 term-well h-[60vh] overflow-y-auto p-4 font-mono text-[13px] leading-relaxed">
                {shown
                  .filter((e) => e.kind === 'command' || e.kind === 'crash')
                  .map((e, i) =>
                    e.kind === 'command' ? (
                      <div key={i} className="mb-2">
                        <div>
                          <span className="text-[#3bb273]">root@entropy</span>
                          <span className="text-[#d7dce2]"># {e.command}</span>
                        </div>
                        {e.output && <pre className="whitespace-pre-wrap text-[#aeb6bf] m-0">{e.output}</pre>}
                        {(e.exitCode ?? 0) !== 0 && <div className="text-[#f97066] opacity-80">[exit {e.exitCode}]</div>}
                      </div>
                    ) : (
                      <div key={i} className="mb-2 text-[#f97066]">
                        crash {e.message}
                      </div>
                    ),
                  )}
                {index >= events.length && <div className="text-[#5c6570]">end of recording</div>}
              </div>
              <ol className="panel p-3 h-[60vh] overflow-y-auto space-y-2">
                {shown
                  .filter((e) => KIND_STYLE[e.kind])
                  .map((e, i) => {
                    const [label, color] = KIND_STYLE[e.kind];
                    return (
                      <li key={i} className="text-sm leading-snug">
                        <div className="flex gap-2 text-[11px]">
                          <span className="tabular-nums text-ink-muted">
                            +{Math.round((at(e) - start) / 1000)}s
                          </span>
                          <span className={`font-medium ${color}`}>{label}</span>
                        </div>
                        <div className="text-ink">{e.kind === 'ai' ? e.message.slice(0, 220) : e.message}</div>
                      </li>
                    );
                  })}
              </ol>
            </div>
          </>
        )}
      </main>
    </div>
  );
}
