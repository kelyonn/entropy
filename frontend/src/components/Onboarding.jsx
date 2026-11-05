const LOOP = [
  {
    step: '01',
    title: 'Break something',
    body: 'Run destructive commands in an isolated Ubuntu container, or start an incident that is already on fire.',
  },
  {
    step: '02',
    title: 'Watch the system',
    body: 'CPU, memory, and process count update live, so you see an OOM kill or a fork storm as it happens.',
  },
  {
    step: '03',
    title: 'Understand why',
    body: 'Crashes come with a root-cause explanation. Incidents check your fix automatically and end with a debrief.',
  },
];

const SAMPLE = [
  ['Processes', 'The process that is not a kernel thread'],
  ['Storage', 'Disk full, but nothing is there'],
  ['Processes', 'Process table exhaustion'],
  ['Permissions', 'Permission denied on deploy'],
  ['Networking', 'Address already in use'],
];

export default function Onboarding({ onStart, error }) {
  return (
    <div className="min-h-screen bg-canvas flex items-center justify-center p-4 md:p-8">
      <div className="panel max-w-4xl w-full grid md:grid-cols-5 overflow-hidden">
        <div className="md:col-span-3 p-6 md:p-10">
          <h1 className="text-3xl font-bold tracking-tight text-accent">ENTROPY</h1>
          <p className="mt-2 text-ink text-base leading-relaxed">
            A flight simulator for production failures. Practice the commands you are afraid to run, on a machine you
            are allowed to break.
          </p>

          <ol className="mt-8 space-y-5">
            {LOOP.map((item) => (
              <li key={item.step} className="flex gap-4">
                <span className="font-mono text-xs text-accent pt-0.5">{item.step}</span>
                <div>
                  <div className="text-sm font-semibold text-ink">{item.title}</div>
                  <p className="text-sm text-ink-muted mt-0.5 leading-relaxed">{item.body}</p>
                </div>
              </li>
            ))}
          </ol>

          {error && <p className="mt-6 text-sm text-danger">{error}</p>}

          <button
            type="button"
            onClick={() => onStart()}
            className="mt-8 w-full sm:w-auto px-6 bg-accent text-white font-medium py-2.5 rounded-panel hover:opacity-90 transition-opacity"
          >
            Start sandbox
          </button>
          <p className="mt-3 text-xs text-ink-muted">
            No account needed. Sandboxes have no network, 256 MB of memory, half a CPU, and end after an hour.
          </p>
        </div>

        <div className="md:col-span-2 bg-muted border-t md:border-t-0 md:border-l border-line p-6 md:p-8 flex flex-col">
          <div className="term-well px-4 py-3 font-mono text-xs leading-relaxed">
            <div>
              <span className="text-[#3bb273]">root@entropy</span>:<span className="text-[#6b9bd1]">~</span># df -h /var/log
            </div>
            <div className="text-[#8b949e]">tmpfs  64M  57M  7.0M  90% /var/log</div>
            <div>
              <span className="text-[#3bb273]">root@entropy</span>:<span className="text-[#6b9bd1]">~</span># du -sh /var/log
            </div>
            <div className="text-[#8b949e]">12K   /var/log</div>
            <div className="text-[#d4a017]">where did 57 MB go?</div>
          </div>
          <div className="mt-6 text-xs font-medium text-ink-muted">Incident labs include</div>
          <ul className="mt-2 divide-y divide-line">
            {SAMPLE.map(([cat, title]) => (
              <li key={title} className="py-2">
                <div className="text-sm text-ink">{title}</div>
                <div className="text-[11px] text-ink-muted">{cat}</div>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  );
}
