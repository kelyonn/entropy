import { useEffect, useRef, useState, useCallback } from 'react';

const defaultProtocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
const WS_URL = import.meta.env.VITE_WS_URL || `${defaultProtocol}://${window.location.host}/ws`;

export function useWebSocket(sessionId, authToken = '') {
  const ws = useRef(null);
  const reconnectAttempts = useRef(0);
  const intentionalClose = useRef(false);
  const outputSeq = useRef(0);
  const [connected, setConnected] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [connectionLost, setConnectionLost] = useState(false);
  const [outputChunk, setOutputChunk] = useState(null);
  const [exitCode, setExitCode] = useState(null);
  const [crashData, setCrashData] = useState(null);
  const [aiExplanation, setAiExplanation] = useState('');
  const [aiHistory, setAiHistory] = useState([]);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [health, setHealth] = useState('healthy');
  const [vizEvent, setVizEvent] = useState(null);
  const [busy, setBusy] = useState(false);
  const [activeScenario, setActiveScenario] = useState(null);
  const [resourceStats, setResourceStats] = useState({ cpu: null, memory: null });
  const [sessionSummary, setSessionSummary] = useState(null);
  const [aiFallback, setAiFallback] = useState(false);

  const handleMessage = useCallback((raw) => {
    let msg;
    try {
      msg = JSON.parse(raw);
    } catch {
      return;
    }

    const type = msg.type;
    const payload = msg.payload ?? msg;

    switch (type) {
      case 'output': {
        const data = payload.stdout ?? payload.data ?? '';
        const stderr = payload.stderr ?? '';
        const text = (data || '') + (stderr || '');
        if (text) {
          outputSeq.current += 1;
          setOutputChunk({ seq: outputSeq.current, text });
        }
        if (payload.completed) {
          setExitCode(payload.exitCode ?? payload.exit_code ?? 0);
          setBusy(false);
          if (payload.exitCode && payload.exitCode !== 0) {
            setHealth('error');
          }
        } else if (text) {
          setBusy(true);
        }
        break;
      }
      case 'crash':
        setCrashData(payload.data ?? payload.message ?? String(payload));
        setHealth('crashed');
        setIsAnalyzing(true);
        setAiExplanation('');
        break;
      case 'visualization':
        if (payload.status) {
          setHealth(payload.status);
          if (payload.status === 'healthy') {
            setVizEvent(null);
          }
        }
        if (payload.event) {
          setVizEvent(payload.event);
        }
        if (payload.cpu != null || payload.memory != null) {
          setResourceStats({
            cpu: payload.cpu ?? null,
            memory: payload.memory ?? null,
          });
        }
        break;
      case 'ai_stream':
        if (payload.reset) {
          setAiExplanation('');
        }
        if (payload.fallback) {
          setAiFallback(true);
        }
        if (payload.done) {
          setIsAnalyzing(false);
          setAiExplanation((prev) => {
            if (prev) {
              setAiHistory((h) => [...h.slice(-9), prev]);
            }
            return prev;
          });
        } else {
          setIsAnalyzing(true);
          const token = payload.token || payload.data || '';
          if (token) {
            setAiExplanation((prev) => prev + token);
          }
        }
        break;
      case 'ai': {
        const text = payload.data ?? payload;
        if (text === 'Analyzing crash...' || text === 'Analyzing failure...') {
          setIsAnalyzing(true);
          setAiExplanation('');
        } else {
          setAiExplanation(text);
          setIsAnalyzing(false);
        }
        break;
      }
      case 'scenario':
        setActiveScenario(payload.active ? payload.id : null);
        break;
      case 'session_summary':
        setSessionSummary(payload);
        break;
      case 'error':
        outputSeq.current += 1;
        setOutputChunk({ seq: outputSeq.current, text: `\r\n${payload.message || 'error'}\r\n` });
        setBusy(false);
        break;
      case 'pong':
      case 'heartbeat':
        break;
      default:
        break;
    }
  }, []);

  const connect = useCallback(() => {
    if (!sessionId) return;
    intentionalClose.current = false;
    setReconnecting(reconnectAttempts.current > 0);
    const params = new URLSearchParams({ session_id: sessionId });
    if (authToken) {
      params.set('access_token', authToken);
    } else {
      params.set('guest', '1');
    }
    const url = `${WS_URL}?${params.toString()}`;
    const socket = new WebSocket(url);
    ws.current = socket;

    socket.onopen = () => {
      setConnected(true);
      setReconnecting(false);
      setConnectionLost(false);
      reconnectAttempts.current = 0;
    };

    socket.onmessage = (event) => handleMessage(event.data);

    socket.onclose = () => {
      setConnected(false);
      setBusy(false);
      if (intentionalClose.current) return;

      if (reconnectAttempts.current < 3) {
        const delay = 1000 * Math.pow(2, reconnectAttempts.current);
        setReconnecting(true);
        reconnectAttempts.current += 1;
        setTimeout(connect, delay);
      } else {
        setReconnecting(false);
        setConnectionLost(true);
      }
    };

    socket.onerror = () => {
      // onclose handles reconnect
    };
  }, [sessionId, authToken, handleMessage]);

  useEffect(() => {
    connect();
    const ping = setInterval(() => {
      if (ws.current?.readyState === WebSocket.OPEN) {
        ws.current.send(JSON.stringify({ type: 'heartbeat', sessionId, payload: {} }));
      }
    }, 30000);

    return () => {
      intentionalClose.current = true;
      clearInterval(ping);
      ws.current?.close();
    };
  }, [connect, sessionId]);

  const inputDisabled = !connected || reconnecting || connectionLost;

  const sendCommand = useCallback(
    (command) => {
      if (inputDisabled || ws.current?.readyState !== WebSocket.OPEN) return;
      setExitCode(null);
      setBusy(true);
      setCrashData(null);
      setAiExplanation('');
      setIsAnalyzing(false);
      setHealth('healthy');
      setVizEvent(null);
      setAiFallback(false);
      ws.current.send(
        JSON.stringify({
          type: 'command',
          sessionId,
          payload: { command, executionType: 'interactive' },
          command,
          session_id: sessionId,
        }),
      );
    },
    [sessionId, inputDisabled],
  );

  const runScenario = useCallback(
    (id) => {
      if (inputDisabled || ws.current?.readyState !== WebSocket.OPEN) return;
      setActiveScenario(id);
      setCrashData(null);
      setAiExplanation('');
      setIsAnalyzing(false);
      setHealth('healthy');
      setVizEvent(null);
      ws.current.send(
        JSON.stringify({
          type: 'scenario',
          sessionId,
          payload: { id, action: 'start' },
        }),
      );
    },
    [sessionId, inputDisabled],
  );

  const interruptScenario = useCallback(() => {
    if (ws.current?.readyState !== WebSocket.OPEN) return;
    ws.current.send(
      JSON.stringify({
        type: 'scenario',
        sessionId,
        payload: { action: 'interrupt' },
      }),
    );
    setActiveScenario(null);
  }, [sessionId]);

  const askAI = useCallback(
    (question, simpler = false) => {
      if (ws.current?.readyState !== WebSocket.OPEN) return;
      setAiExplanation('');
      setIsAnalyzing(true);
      ws.current.send(
        JSON.stringify({
          type: 'ai_question',
          sessionId,
          payload: { question, simpler },
        }),
      );
    },
    [sessionId],
  );

  return {
    connected,
    reconnecting,
    connectionLost,
    outputChunk,
    exitCode,
    crashData,
    aiExplanation,
    aiHistory,
    isAnalyzing,
    health,
    vizEvent,
    busy,
    activeScenario,
    resourceStats,
    sessionSummary,
    aiFallback,
    inputDisabled,
    sendCommand,
    runScenario,
    interruptScenario,
    askAI,
  };
}
