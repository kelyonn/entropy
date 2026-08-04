const EXAMPLES = ['rm -rf /', 'kill -9 1', ':(){ :|:& };:'];

export default function Onboarding({ onStart, error }) {
  return (
    <div className="min-h-screen bg-canvas flex items-center justify-center p-6">
      <div className="panel max-w-lg w-full p-8">
        <h1 className="text-3xl font-bold tracking-tight text-accent">ENTROPY</h1>
        <p className="mt-2 text-ink-muted text-sm">
          A safe sandbox for practicing dangerous system commands and learning from failures.
        </p>

        <div className="mt-6 space-y-3 text-sm text-ink">
          <p>You get an isolated container, a live terminal, and explanations when things break.</p>
          <p className="font-medium">All commands run in isolation.</p>
        </div>

        <div className="mt-6">
          <div className="text-xs font-medium text-ink-muted mb-2">Example commands</div>
          <div className="flex flex-wrap gap-2">
            {EXAMPLES.map((cmd) => (
              <code
                key={cmd}
                className="font-mono text-xs bg-muted text-ink px-2 py-1 rounded-panel border border-line"
              >
                {cmd}
              </code>
            ))}
          </div>
        </div>

        {error && <p className="mt-4 text-sm text-danger">{error}</p>}

        <button
          type="button"
          onClick={onStart}
          className="mt-8 w-full bg-accent text-white font-medium py-2.5 rounded-panel hover:opacity-90 transition-opacity"
        >
          Start sandbox
        </button>
      </div>
    </div>
  );
}
