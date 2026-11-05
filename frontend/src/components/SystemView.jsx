import { useMemo, useRef } from 'react';
import { Canvas, useFrame } from '@react-three/fiber';
import * as THREE from 'three';
import Sparkline from './Sparkline';

const PID_LIMIT = 100;
const COLORS = {
  ok: new THREE.Color('#1f6b5c'),
  warn: new THREE.Color('#a15c07'),
  danger: new THREE.Color('#b42318'),
  base: new THREE.Color('#d9dde3'),
  baseCrash: new THREE.Color('#f3d6d3'),
  baseAnalyze: new THREE.Color('#f2e4cf'),
};

// Outline of a full column, so each bar reads as a gauge against its limit.
const CAPACITY_EDGES = new THREE.EdgesGeometry(new THREE.BoxGeometry(0.62, 2.2, 0.62));

function levelColor(pct) {
  if (pct >= 85) return COLORS.danger;
  if (pct >= 60) return COLORS.warn;
  return COLORS.ok;
}

// Column is one resource bar. Height eases toward the live value; events
// add motion on the column they relate to.
function Column({ x, pct, shake }) {
  const mesh = useRef();
  const mat = useRef();
  const current = useRef(0.02);
  useFrame((state, delta) => {
    const target = Math.max(0.02, Math.min(pct, 100) / 100);
    current.current += (target - current.current) * Math.min(1, delta * 4);
    const h = current.current * 2.2;
    if (mesh.current) {
      mesh.current.scale.y = h;
      mesh.current.position.y = h / 2;
      mesh.current.position.x = x + (shake ? Math.sin(state.clock.elapsedTime * 42) * 0.035 : 0);
    }
    if (mat.current) {
      mat.current.color.lerp(levelColor(pct), Math.min(1, delta * 5));
      mat.current.emissiveIntensity = shake ? 0.35 + Math.sin(state.clock.elapsedTime * 12) * 0.2 : 0.08;
    }
  });
  return (
    <group>
      <lineSegments position={[x, 1.1, 0]} geometry={CAPACITY_EDGES}>
        <lineBasicMaterial color="#5c6570" transparent opacity={0.28} />
      </lineSegments>
      <mesh ref={mesh} position={[x, 0.01, 0]} castShadow>
        <boxGeometry args={[0.55, 1, 0.55]} />
        <meshStandardMaterial ref={mat} color="#1f6b5c" emissive="#b42318" emissiveIntensity={0.08} roughness={0.55} />
      </mesh>
      <mesh position={[x, 0.005, 0]} rotation={[-Math.PI / 2, 0, 0]}>
        <planeGeometry args={[0.7, 0.7]} />
        <meshBasicMaterial color="#c4c9d1" transparent opacity={0.6} />
      </mesh>
    </group>
  );
}

function Rack({ cpu, memory, pidsPct, health, event }) {
  const group = useRef();
  const base = useRef();
  useFrame((state, delta) => {
    const t = state.clock.elapsedTime;
    if (group.current) {
      const crash = health === 'crashed' && event === 'kernel_panic';
      group.current.rotation.y = Math.sin(t * 0.25) * 0.12;
      group.current.position.x = crash ? Math.sin(t * 50) * 0.05 : 0;
    }
    if (base.current) {
      const target =
        health === 'crashed' || health === 'error'
          ? COLORS.baseCrash
          : health === 'analyzing'
            ? COLORS.baseAnalyze
            : COLORS.base;
      base.current.color.lerp(target, Math.min(1, delta * 3));
    }
  });
  return (
    <group ref={group} position={[0, -1.1, 0]}>
      <mesh position={[0, -0.08, 0]} receiveShadow>
        <boxGeometry args={[3.1, 0.14, 1.3]} />
        <meshStandardMaterial ref={base} color="#d9dde3" roughness={0.9} />
      </mesh>
      <Column x={-1} pct={cpu} shake={event === 'timeout'} />
      <Column x={0} pct={memory} shake={event === 'oom_kill'} />
      <Column x={1} pct={pidsPct} shake={event === 'segfault' || event === 'kernel_panic'} />
    </group>
  );
}

function formatBytes(bytes) {
  if (bytes == null) return null;
  return `${Math.round(bytes / (1024 * 1024))} MB`;
}

const HEALTH_LABEL = {
  healthy: ['Healthy', 'text-success'],
  error: ['Last command failed', 'text-danger'],
  crashed: ['Crashed', 'text-danger'],
  analyzing: ['Analyzing', 'text-warning'],
};

const EVENT_LABEL = {
  oom_kill: 'OOM kill',
  segfault: 'segfault',
  kernel_panic: 'init failure',
  timeout: 'timeout',
};

export default function SystemView({ health = 'healthy', event, samples, latest }) {
  const cpu = latest.cpu ?? 0;
  const memory = latest.memory ?? 0;
  const pids = latest.pids ?? 0;
  const pidsPct = (pids / PID_LIMIT) * 100;
  const series = useMemo(
    () => ({
      cpu: samples.map((s) => s.cpu),
      memory: samples.map((s) => s.memory),
      pids: samples.map((s) => s.pids ?? 0),
    }),
    [samples],
  );
  const [healthText, healthColor] = HEALTH_LABEL[health] || HEALTH_LABEL.healthy;
  const memDetail =
    latest.memoryBytes != null ? `${formatBytes(latest.memoryBytes)} of ${formatBytes(latest.memoryLimit)}` : 'of 256 MB';

  const stats = [
    { key: 'cpu', label: 'CPU', value: `${cpu}%`, detail: 'of 0.5 core quota', pct: cpu, series: series.cpu, max: 100 },
    { key: 'memory', label: 'Memory', value: `${memory}%`, detail: memDetail, pct: memory, series: series.memory, max: 100 },
    { key: 'pids', label: 'Processes', value: `${pids}`, detail: `of ${PID_LIMIT} allowed`, pct: pidsPct, series: series.pids, max: PID_LIMIT },
  ];

  return (
    <section className="panel">
      <header className="flex items-center justify-between px-4 pt-3">
        <h2 className="text-sm font-semibold text-ink">System view</h2>
        <span className={`text-xs font-medium status-fade ${healthColor}`}>
          {healthText}
          {event && EVENT_LABEL[event] ? ` (${EVENT_LABEL[event]})` : ''}
        </span>
      </header>
      <div className="relative h-40 mx-4 mt-2 rounded-panel bg-muted overflow-hidden">
        <div className="absolute inset-x-0 bottom-1.5 z-10 grid grid-cols-3 text-center text-[10px] font-mono text-ink-muted pointer-events-none">
          <span>cpu</span>
          <span>mem</span>
          <span>pids</span>
        </div>
        <Canvas camera={{ position: [0, 0.9, 3.6], fov: 42 }} dpr={[1, 2]} onCreated={({ camera }) => camera.lookAt(0, 0, 0)}>
          <ambientLight intensity={0.75} />
          <directionalLight position={[3, 5, 4]} intensity={0.9} />
          <Rack cpu={cpu} memory={memory} pidsPct={pidsPct} health={health} event={event} />
        </Canvas>
      </div>
      <div className="grid grid-cols-3 gap-px bg-line mt-3 border-t border-line rounded-b-panel overflow-hidden">
        {stats.map((s) => {
          const color = s.pct >= 85 ? 'var(--danger)' : s.pct >= 60 ? 'var(--warning)' : 'var(--accent)';
          return (
            <div key={s.key} className="bg-surface px-3 pt-2 pb-1.5">
              <div className="text-[11px] text-ink-muted">{s.label}</div>
              <div className="text-base font-semibold tabular-nums" style={{ color }}>
                {latest.cpu == null ? '--' : s.value}
              </div>
              <div className="text-[11px] text-ink-muted truncate">{s.detail}</div>
              <div className="mt-1">
                <Sparkline values={s.series} max={s.max} color={color} label={`${s.label} history`} />
              </div>
            </div>
          );
        })}
      </div>
      {latest.traffic && <TrafficRow samples={samples} traffic={latest.traffic} />}
    </section>
  );
}

// TrafficRow shows the production stack's live service levels during
// stack labs: requests per second, error rate, and p95 latency.
function TrafficRow({ samples, traffic }) {
  const series = samples.filter((s) => s.traffic);
  const errColor = traffic.errorPct >= 1 ? 'var(--danger)' : 'var(--accent)';
  const latColor = traffic.p95 >= 150 ? 'var(--danger)' : traffic.p95 >= 100 ? 'var(--warning)' : 'var(--accent)';
  const cells = [
    { key: 'rps', label: 'Requests', value: `${traffic.rps}/s`, detail: 'load generator', values: series.map((s) => s.traffic.rps), max: 32, color: 'var(--accent)' },
    { key: 'err', label: 'Errors', value: `${traffic.errorPct.toFixed(1)}%`, detail: 'SLO below 1%', values: series.map((s) => s.traffic.errorPct), max: 100, color: errColor },
    { key: 'p95', label: 'p95 latency', value: `${traffic.p95} ms`, detail: 'SLO below 150 ms', values: series.map((s) => Math.min(s.traffic.p95, 1000)), max: 1000, color: latColor },
  ];
  return (
    <div className="border-t border-line">
      <div className="px-4 pt-2 text-[11px] font-medium text-ink-muted">Production traffic</div>
      <div className="grid grid-cols-3 gap-px bg-line mt-1 rounded-b-panel overflow-hidden">
        {cells.map((c) => (
          <div key={c.key} className="bg-surface px-3 pt-1.5 pb-1.5">
            <div className="text-[11px] text-ink-muted">{c.label}</div>
            <div className="text-base font-semibold tabular-nums" style={{ color: c.color }}>
              {c.value}
            </div>
            <div className="text-[11px] text-ink-muted truncate">{c.detail}</div>
            <div className="mt-1">
              <Sparkline values={c.values} max={c.max} color={c.color} label={`${c.label} history`} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
