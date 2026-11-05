function formatDuration(ms) {
  if (ms == null) return '';
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(1)} s`;
}

function formatTime(at) {
  return new Date(at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' });
}

// Timeline lists recent commands, crashes, and lab milestones, newest first.
export default function Timeline({ entries, onExplain }) {
  const items = [...entries].reverse();
  // Only the most recent failure can be explained; the server keeps its output.
  const latestFailure = items.find((e) => e.kind === 'command' && e.exitCode !== 0);
  return (
    <section className="panel flex flex-col min-h-0">
      <header className="flex items-center justify-between px-4 py-2.5 border-b border-line">
        <h2 className="text-sm font-semibold text-ink">Activity</h2>
        <span className="text-xs text-ink-muted tabular-nums">
          {entries.filter((e) => e.kind === 'command').length} commands
        </span>
      </header>
      {items.length === 0 ? (
        <p className="px-4 py-3 text-xs text-ink-muted">Commands you run appear here with exit status and duration.</p>
      ) : (
        <ol className="overflow-y-auto min-h-0 divide-y divide-line">
          {items.map((e) => {
            const failed = e.kind === 'crash' || (e.kind === 'command' && e.exitCode !== 0);
            return (
              <li key={e.id} className="px-4 py-1.5 flex items-center gap-3 text-xs">
                <span className="text-ink-muted tabular-nums shrink-0 w-16">{formatTime(e.at)}</span>
                <span
                  className={`shrink-0 w-16 whitespace-nowrap font-mono ${
                    e.kind === 'lab' ? 'text-accent' : failed ? 'text-danger' : 'text-success'
                  }`}
                >
                  {e.kind === 'lab' ? 'lab' : e.kind === 'crash' ? 'crash' : e.exitCode === 0 ? 'ok' : `exit ${e.exitCode}`}
                </span>
                <span className="font-mono text-ink truncate flex-1" title={e.command}>
                  {e.kind === 'lab' ? `${e.message}: ${e.command}` : e.command?.split('\n')[0]}
                </span>
                <span className="text-ink-muted tabular-nums shrink-0">{formatDuration(e.durationMs)}</span>
                {latestFailure && e.id === latestFailure.id && onExplain && (
                  <button type="button" onClick={() => onExplain(e)} className="text-accent font-medium shrink-0">
                    Explain
                  </button>
                )}
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}
