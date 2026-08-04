export default function StatusBar({ connected, reconnecting, connectionLost, sessionId }) {
  let label = 'Offline';
  let color = 'text-ink-muted';

  if (connectionLost) {
    label = 'Connection lost';
    color = 'text-danger';
  } else if (reconnecting) {
    label = 'Reconnecting';
    color = 'text-warning';
  } else if (connected) {
    label = 'Connected';
    color = 'text-success';
  }

  return (
    <div className="flex items-center gap-4 text-sm">
      <span className={`status-fade font-medium ${color}`}>{label}</span>
      {sessionId && (
        <span className="font-mono text-xs text-ink-muted">
          session {sessionId.slice(0, 8)}
        </span>
      )}
    </div>
  );
}
