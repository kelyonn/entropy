export default function MetricTile({ label, value }) {
  return (
    <div className="panel px-3 py-2.5">
      <div className="text-xl font-semibold text-accent tabular-nums">{value ?? '0'}</div>
      <div className="text-xs text-ink-muted mt-0.5">{label}</div>
    </div>
  );
}
