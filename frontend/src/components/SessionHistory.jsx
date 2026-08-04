export default function SessionHistory({ sessions, onRestore }) {
  if (!sessions?.length) {
    return (
      <div className="panel p-3">
        <h2 className="text-sm font-semibold text-ink mb-2">Saved sessions</h2>
        <p className="text-xs text-ink-muted">Sign in to see session history.</p>
      </div>
    );
  }

  return (
    <div className="panel p-3">
      <h2 className="text-sm font-semibold text-ink mb-2">Saved sessions</h2>
      <ul className="space-y-2">
        {sessions.map((s) => (
          <li key={s.id} className="flex items-center justify-between gap-2 text-xs">
            <div>
              <div className="font-mono text-ink">{s.id.slice(0, 8)}</div>
              <div className="text-ink-muted">{s.created_at}</div>
            </div>
            <button
              type="button"
              onClick={() => onRestore(s.id)}
              className="text-accent font-medium"
            >
              Restore
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
