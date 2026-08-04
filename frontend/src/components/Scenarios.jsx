import { useState } from 'react';

const SCENARIOS = [
  { id: 'cpu', name: 'High CPU', description: 'Sustained CPU pressure inside the sandbox.', context: 'This scenario runs openssl speed workers to spike CPU. Watch the metric tile and system view.' },
  { id: 'fork', name: 'Fork bomb', description: 'Process spawn storm, limited by PID cgroup.', context: 'A recursive fork pattern hits the PID limit (100). The sandbox should survive; the host does not.' },
  { id: 'oom', name: 'OOM', description: 'Memory pressure beyond the 256 MB hard limit.', context: 'Allocates memory until the cgroup OOM killer terminates the process (exit 137).' },
  { id: 'panic', name: 'Kill init', description: 'Signal PID 1 and observe container failure.', context: 'Attempts to kill PID 1 inside the container. May be blocked; simulates init failure paths.' },
];

export default function Scenarios({ onRun, onInterrupt, activeId, disabled }) {
  const [pending, setPending] = useState(null);

  return (
    <div className="panel p-3">
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-sm font-semibold text-ink">Scenarios</h2>
        {activeId && (
          <button
            type="button"
            onClick={onInterrupt}
            className="text-xs text-danger font-medium"
            disabled={disabled}
          >
            Interrupt
          </button>
        )}
      </div>
      <ul className="space-y-2">
        {SCENARIOS.map((s) => (
          <li key={s.id}>
            <button
              type="button"
              disabled={disabled || !!activeId}
              onClick={() => setPending(s)}
              className={`w-full text-left px-3 py-2 rounded-panel border border-line transition-colors ${
                activeId === s.id ? 'bg-accent-soft' : 'bg-surface hover:bg-muted'
              } disabled:opacity-50`}
            >
              <div className="text-sm font-medium text-ink">{s.name}</div>
              <div className="text-xs text-ink-muted mt-0.5">{s.description}</div>
            </button>
          </li>
        ))}
      </ul>

      {pending && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4">
          <div className="panel max-w-md w-full p-6">
            <h3 className="text-lg font-semibold text-ink">{pending.name}</h3>
            <p className="mt-2 text-sm text-ink-muted">{pending.context}</p>
            <div className="mt-6 flex gap-2 justify-end">
              <button
                type="button"
                onClick={() => setPending(null)}
                className="text-sm px-4 py-2 rounded-panel border border-line"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={() => {
                  onRun(pending.id);
                  setPending(null);
                }}
                className="text-sm px-4 py-2 rounded-panel bg-accent text-white"
              >
                Run scenario
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
