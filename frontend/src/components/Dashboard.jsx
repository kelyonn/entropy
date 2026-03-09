import { useState, useEffect } from 'react';
import axios from 'axios';

const API_URL = import.meta.env.VITE_API_URL || '';

export default function Dashboard({ sessionId }) {
  const [metrics, setMetrics] = useState({});

  useEffect(() => {
    const fetch = () =>
      axios.get(`${API_URL}/api/dashboard`)
        .then(res => setMetrics(res.data));

    fetch();
    const interval = setInterval(fetch, 5000);
    return () => clearInterval(interval);
  }, []);

  const stats = [
    { label: 'Sessions', key: 'metric:total_sessions', icon: '🖥️' },
    { label: 'Commands', key: 'metric:total_commands', icon: '⌨️' },
    { label: 'Crashes', key: 'metric:total_crashes', icon: '💥' },
    { label: 'AI Calls', key: 'metric:ai_explanations', icon: '🤖' },
  ];

  return (
    <div className="grid grid-cols-2 gap-2">
      {stats.map(stat => (
        <div key={stat.key}
          className="bg-gray-800 rounded-lg p-3 border border-gray-700">
          <div className="text-lg">{stat.icon}</div>
          <div className="text-2xl font-bold text-blue-400">
            {metrics[stat.key] || '0'}
          </div>
          <div className="text-xs text-gray-400">{stat.label}</div>
        </div>
      ))}
    </div>
  );
}
