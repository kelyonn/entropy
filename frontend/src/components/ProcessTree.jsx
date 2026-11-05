import { useEffect, useMemo, useRef, useState } from 'react';

const STATE = {
  R: ['run', 'text-success'],
  S: ['sleep', 'text-ink-muted'],
  D: ['disk', 'text-warning'],
  Z: ['zombie', 'text-warning'],
  T: ['stopped', 'text-warning'],
  I: ['idle', 'text-ink-muted'],
};

const GROUP_AT = 6;
const GHOST_MS = 3500;

const INTERPRETERS = new Set(['bash', 'sh', 'dash', 'python3', 'python', 'perl']);

// name shows what a learner recognizes: the script for interpreter
// processes (spoold, stack.py db), the program otherwise.
function name(p) {
  const argv = (p.args || '').split(' ').filter(Boolean);
  const base = (argv[0] || '').split('/').pop();
  if (!base || base.startsWith('[')) return p.comm;
  if (INTERPRETERS.has(base) && argv[1] && !argv[1].startsWith('-')) {
    const script = argv[1].split('/').pop();
    return argv[2] && !argv[2].startsWith('-') && script === 'stack.py' ? `${script} ${argv[2]}` : script;
  }
  return base;
}

function fmtKB(kb) {
  if (kb >= 1024) return `${Math.round(kb / 1024)} MB`;
  return `${kb} KB`;
}

// buildRows turns a flat process list into pstree-style rows. Large groups of
// identical leaf siblings (fork storms, worker pools) collapse into one row.
function buildRows(procs) {
  const byPid = new Map(procs.map((p) => [p.pid, p]));
  const kids = new Map();
  procs.forEach((p) => {
    const parent = byPid.has(p.ppid) ? p.ppid : 0;
    if (!kids.has(parent)) kids.set(parent, []);
    kids.get(parent).push(p);
  });
  const rows = [];
  const walk = (pid, depth, prefix) => {
    const children = (kids.get(pid) || []).slice().sort((a, b) => a.pid - b.pid);
    const leaves = new Map();
    const items = [];
    children.forEach((c) => {
      const isLeaf = !(kids.get(c.pid) || []).length;
      const key = `${name(c)}|${c.state === 'Z' ? 'Z' : ''}`;
      if (isLeaf) {
        if (!leaves.has(key)) leaves.set(key, []);
        leaves.get(key).push(c);
      } else {
        items.push({ proc: c });
      }
    });
    leaves.forEach((group) => {
      if (group.length >= GROUP_AT) items.push({ group });
      else group.forEach((proc) => items.push({ proc }));
    });
    items.sort((a, b) => (a.proc || a.group[0]).pid - (b.proc || b.group[0]).pid);
    items.forEach((item, i) => {
      const last = i === items.length - 1;
      const branch = depth === 0 ? '' : `${prefix}${last ? '└─ ' : '├─ '}`;
      if (item.group) {
        rows.push({ key: `g${item.group[0].pid}`, branch, group: item.group });
      } else {
        rows.push({ key: `p${item.proc.pid}`, branch, proc: item.proc });
        walk(item.proc.pid, depth + 1, depth === 0 ? '' : `${prefix}${last ? '   ' : '│  '}`);
      }
    });
  };
  walk(0, 0, '');
  return rows;
}

export default function ProcessTree({ snapshot, crash }) {
  const [ghosts, setGhosts] = useState([]);
  const [fresh, setFresh] = useState(new Set());
  const [selected, setSelected] = useState(null);
  const prev = useRef(null);

  useEffect(() => {
    if (!snapshot) return;
    const now = Date.now();
    const current = new Map(snapshot.procs.map((p) => [p.pid, p]));
    if (prev.current) {
      const gone = [...prev.current.values()].filter((p) => !current.has(p.pid));
      const born = snapshot.procs.filter((p) => !prev.current.has(p.pid)).map((p) => p.pid);
      if (gone.length) {
        setGhosts((g) => [...g.filter((x) => now - x.at < GHOST_MS), ...gone.map((p) => ({ ...p, at: now }))].slice(-12));
      }
      setFresh(new Set(born));
    }
    prev.current = current;
  }, [snapshot]);

  useEffect(() => {
    if (!ghosts.length) return undefined;
    const t = setTimeout(() => setGhosts((g) => g.filter((x) => Date.now() - x.at < GHOST_MS)), 800);
    return () => clearTimeout(t);
  }, [ghosts]);

  // After an OOM kill, the largest process that just vanished is the victim.
  const oomVictim = useMemo(() => {
    if (!crash || crash.event !== 'oom_kill' || Date.now() - crash.at > 8000) return null;
    const recent = ghosts.filter((g) => Math.abs(g.at - crash.at) < 6000);
    return recent.sort((a, b) => b.rssKb - a.rssKb)[0]?.pid ?? null;
  }, [crash, ghosts]);

  if (!snapshot) {
    return <div className="h-56 flex items-center justify-center text-xs text-ink-muted">Reading /proc...</div>;
  }

  const rows = buildRows(snapshot.procs);
  const zombies = snapshot.procs.filter((p) => p.state === 'Z').length;
  const running = snapshot.procs.filter((p) => p.state === 'R').length;
  const top = [...snapshot.procs].sort((a, b) => b.cpu - a.cpu)[0];
  const sel = selected && snapshot.procs.find((p) => p.pid === selected);

  return (
    <div>
      <div className="flex flex-wrap gap-x-4 gap-y-1 px-1 pb-2 text-[11px] text-ink-muted tabular-nums">
        <span>{snapshot.procs.length} processes</span>
        <span className={running ? 'text-success' : ''}>{running} running</span>
        <span className={zombies ? 'text-warning font-medium' : ''}>{zombies} zombie</span>
        {top && top.cpu >= 5 && (
          <span>
            busiest: {name(top)} {Math.round(top.cpu)}%
          </span>
        )}
      </div>
      <div className="h-56 overflow-auto rounded-panel bg-muted px-2 py-1.5 font-mono text-[11.5px] leading-[1.55]">
        {rows.map((r) => {
          if (r.group) {
            const g = r.group;
            const rss = g.reduce((a, p) => a + p.rssKb, 0);
            const zombie = g[0].state === 'Z';
            return (
              <div key={r.key} className="flex items-center whitespace-pre">
                <span className="text-ink-muted">{r.branch}</span>
                <span className={zombie ? 'text-warning font-medium' : 'text-ink'}>
                  {g.length} x {name(g[0])}
                </span>
                <span className="ml-2 text-ink-muted">
                  pids {g[0].pid}-{g[g.length - 1].pid} {zombie ? 'zombie' : fmtKB(rss)}
                </span>
              </div>
            );
          }
          const p = r.proc;
          const [label, color] = STATE[p.state] || [p.state, 'text-ink-muted'];
          const isNew = fresh.has(p.pid);
          return (
            <button
              type="button"
              key={r.key}
              onClick={() => setSelected(selected === p.pid ? null : p.pid)}
              className={`w-full text-left flex items-center whitespace-pre rounded-[3px] ${
                isNew ? 'proc-new' : ''
              } ${selected === p.pid ? 'bg-accent-soft' : 'hover:bg-surface'}`}
              title={p.args}
            >
              <span className="text-ink-muted">{r.branch}</span>
              <span className={p.state === 'Z' ? 'text-warning font-medium' : 'text-ink'}>{name(p)}</span>
              <span className="ml-1.5 text-ink-muted">{p.pid}</span>
              <span className={`ml-1.5 ${color}`}>{label}</span>
              <span className="ml-auto flex items-center gap-2 pl-2 text-ink-muted">
                <span className="w-10 h-1 bg-surface rounded-[2px] overflow-hidden" aria-hidden="true">
                  <span
                    className="block h-full"
                    style={{
                      width: `${Math.min(100, p.cpu * 2)}%`,
                      background: p.cpu >= 40 ? 'var(--danger)' : 'var(--accent)',
                    }}
                  />
                </span>
                <span className="w-12 text-right">{fmtKB(p.rssKb)}</span>
              </span>
            </button>
          );
        })}
        {ghosts.map((g) => (
          <div key={`x${g.pid}-${g.at}`} className="proc-gone flex items-center whitespace-pre text-danger">
            <span>{'x  '}</span>
            <span>{name(g)}</span>
            <span className="ml-1.5 opacity-70">{g.pid}</span>
            <span className="ml-1.5">{oomVictim === g.pid ? 'OOM victim' : 'exited'}</span>
            <span className="ml-auto opacity-70">{fmtKB(g.rssKb)}</span>
          </div>
        ))}
      </div>
      {sel && (
        <div className="mt-2 text-[11px] text-ink-muted px-1">
          <span className="font-mono text-ink break-all">{sel.args}</span>
          <div>
            pid {sel.pid}, parent {sel.ppid}, state {sel.state}, {fmtKB(sel.rssKb)}, cpu {sel.cpu.toFixed(0)}%
          </div>
        </div>
      )}
    </div>
  );
}
