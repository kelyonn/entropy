import { useState, useEffect } from 'react';
import axios from 'axios';
import MetricTile from './MetricTile';

const API_URL = import.meta.env.VITE_API_URL || '';

export default function Dashboard() {
  const [metrics, setMetrics] = useState({});

  useEffect(() => {
    const load = () =>
      axios.get(`${API_URL}/api/dashboard`).then((res) => setMetrics(res.data)).catch(() => {});

    load();
    const interval = setInterval(load, 5000);
    return () => clearInterval(interval);
  }, []);

  const stats = [
    { label: 'Sessions', key: 'metric:total_sessions' },
    { label: 'Commands', key: 'metric:total_commands' },
    { label: 'Crashes', key: 'metric:total_crashes' },
    { label: 'Explanations', key: 'metric:ai_explanations' },
  ];

  return (
    <div className="grid grid-cols-2 gap-2">
      {stats.map((stat) => (
        <MetricTile key={stat.key} label={stat.label} value={metrics[stat.key] || '0'} />
      ))}
    </div>
  );
}
