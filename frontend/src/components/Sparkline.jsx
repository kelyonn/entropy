// Sparkline draws a small area chart of recent samples on a fixed 0-max scale
// so the shape is comparable over time.
export default function Sparkline({ values, max = 100, color = 'var(--accent)', height = 28, label }) {
  const width = 120;
  const pts = values.length > 1 ? values : [0, ...values, 0].slice(-2);
  const step = width / Math.max(1, pts.length - 1);
  const y = (v) => height - (Math.min(Math.max(v ?? 0, 0), max) / max) * (height - 2) - 1;
  const line = pts.map((v, i) => `${(i * step).toFixed(1)},${y(v).toFixed(1)}`).join(' ');
  const area = `0,${height} ${line} ${width},${height}`;
  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className="w-full"
      style={{ height }}
      role="img"
      aria-label={label}
    >
      <polygon points={area} fill={color} opacity="0.12" />
      <polyline points={line} fill="none" stroke={color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}
