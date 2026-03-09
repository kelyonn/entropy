import { useEffect, useRef, useState, useCallback } from 'react';

const defaultProtocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
const WS_URL = import.meta.env.VITE_WS_URL || `${defaultProtocol}://${window.location.host}/ws`;

export function useWebSocket(sessionId) {
  const ws = useRef(null);
  const [connected, setConnected] = useState(false);
  const [output, setOutput] = useState('');
  const [crashData, setCrashData] = useState(null);
  const [aiExplanation, setAiExplanation] = useState('');
  const [isAnalyzing, setIsAnalyzing] = useState(false);

  const connect = useCallback(() => {
    if (!sessionId) {
      return;
    }
    const url = `${WS_URL}?session_id=${sessionId}`;
    ws.current = new WebSocket(url);

    ws.current.onopen = () => {
      setConnected(true);
      console.log('Entropy WebSocket connected');
    };

    ws.current.onmessage = (event) => {
      const msg = JSON.parse(event.data);

      switch (msg.type) {
        case 'output':
          setOutput(prev => prev + msg.data);
          break;

        case 'crash':
          setCrashData(msg.data);
          setIsAnalyzing(true);
          break;

        case 'ai':
          if (msg.data === '🤖 Analyzing crash...') {
            setIsAnalyzing(true);
          } else {
            setAiExplanation(msg.data);
            setIsAnalyzing(false);
          }
          break;

        case 'restore_ok':
          setOutput('✅ Session restored successfully!\n');
          setCrashData(null);
          setAiExplanation('');
          break;

        case 'pong':
          break;
      }
    };

    ws.current.onclose = () => {
      setConnected(false);
      // Auto-reconnect after 3 seconds
      setTimeout(connect, 3000);
    };

    ws.current.onerror = (err) => {
      console.error('WebSocket error:', err);
    };
  }, [sessionId]);

  useEffect(() => {
    connect();
    // Heartbeat ping every 30 seconds
    const ping = setInterval(() => {
      if (ws.current?.readyState === WebSocket.OPEN) {
        ws.current.send(JSON.stringify({ type: 'ping' }));
      }
    }, 30000);

    return () => {
      clearInterval(ping);
      ws.current?.close();
    };
  }, [connect]);

  const sendCommand = useCallback((command) => {
    if (ws.current?.readyState === WebSocket.OPEN) {
      ws.current.send(JSON.stringify({
        type: 'command',
        command,
        session_id: sessionId,
      }));
    }
  }, [sessionId]);

  return {
    connected,
    output,
    crashData,
    aiExplanation,
    isAnalyzing,
    sendCommand,
  };
}
