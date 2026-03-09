import { useState, useEffect } from 'react';
import Terminal from './components/Terminal';
import AIPanel from './components/AIPanel';
import ServerViz from './components/ServerViz';
import Dashboard from './components/Dashboard';
import { useWebSocket } from './hooks/useWebSocket';
import axios from 'axios';

const API_URL = import.meta.env.VITE_API_URL || '';

export default function App() {
  const [sessionId, setSessionId] = useState(null);

  useEffect(() => {
    // Get session_id from URL or create new
    const params = new URLSearchParams(window.location.search);
    const existing = params.get('session_id');

    if (existing) {
      setSessionId(existing);
    } else {
      axios.get(`${API_URL}/api/session/new`).then(res => {
        setSessionId(res.data.session_id);
        window.history.pushState({}, '', `?session_id=${res.data.session_id}`);
      });
    }
  }, []);

  const {
    connected, output, crashData,
    aiExplanation, isAnalyzing, sendCommand,
  } = useWebSocket(sessionId);

  if (!sessionId) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <div className="text-white text-xl animate-pulse">
          ⚡ Spawning sandbox...
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-950 text-white p-4">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <span className="text-2xl font-bold text-blue-400">⚡ ENTROPY</span>
          <span className="text-gray-500 text-sm">crash & learn</span>
        </div>
        <span className="text-xs text-gray-500">
          session: {sessionId?.slice(0, 8)}...
        </span>
      </div>

      {/* Main layout */}
      <div className="grid grid-cols-3 gap-4 h-[calc(100vh-120px)]">
        {/* Terminal - 2/3 width */}
        <div className="col-span-2 flex flex-col gap-4">
          <ServerViz crashed={!!crashData} analyzing={isAnalyzing}/>
          <div className="flex-1">
            <Terminal
              sendCommand={sendCommand}
              output={output}
              crashData={crashData}
              connected={connected}
            />
          </div>
        </div>

        {/* AI Panel + Dashboard - 1/3 width */}
        <div className="flex flex-col gap-4">
          <Dashboard sessionId={sessionId}/>
          <div className="flex-1">
            <AIPanel
              explanation={aiExplanation}
              isAnalyzing={isAnalyzing}
              crashData={crashData}
              sessionId={sessionId}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
