const STEPS = [
  { id: 'sent', label: 'Sent' },
  { id: 'running', label: 'Running in sandbox' },
  { id: 'exited', label: 'Exited' },
  { id: 'explaining', label: 'Explaining' },
];

function stepIndex({ busy, exitCode, isAnalyzing, crashData }) {
  if (isAnalyzing || (crashData && exitCode != null && exitCode !== 0)) {
    return 3;
  }
  if (exitCode != null) {
    return 2;
  }
  if (busy) {
    return 1;
  }
  return 0;
}

export default function CommandFlow({ busy, exitCode, isAnalyzing, crashData }) {
  const active = stepIndex({ busy, exitCode, isAnalyzing, crashData });
  const exitLabel =
    exitCode != null ? (exitCode === 0 ? 'Exited (ok)' : `Exited (code ${exitCode})`) : 'Exited';

  return (
    <div className="panel px-3 py-2 mb-3">
      <div className="text-xs font-medium text-ink-muted mb-2">Command flow</div>
      <ol className="flex flex-wrap items-center gap-2">
        {STEPS.map((step, idx) => {
          const isActive = idx === active;
          const isDone = idx < active;
          const label = step.id === 'exited' && exitCode != null ? exitLabel : step.label;
          return (
            <li key={step.id} className="flex items-center gap-2">
              <span
                className={`text-xs font-mono px-2 py-1 rounded-panel border ${
                  isActive
                    ? 'border-accent text-accent bg-accent-soft'
                    : isDone
                      ? 'border-line text-ink-muted'
                      : 'border-line text-ink-muted opacity-60'
                }`}
              >
                {label}
              </span>
              {idx < STEPS.length - 1 && (
                <span className="text-ink-muted text-xs" aria-hidden="true">
                  →
                </span>
              )}
            </li>
          );
        })}
      </ol>
    </div>
  );
}
