import { useEffect, useRef, useState, useCallback } from 'react';

const defaultProtocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
const WS_URL = import.meta.env.VITE_WS_URL || `${defaultProtocol}://${window.location.host}/ws`;

const MAX_SAMPLES = 90;
const MAX_TIMELINE = 60;

let timelineSeq = 0;
const nextId = () => {
  timelineSeq += 1;
  return timelineSeq;
};

// useWebSocket owns the session connection. Terminal output is delivered
// through a subscriber instead of React state so no chunk is ever dropped by
// batched renders.
export function useWebSocket(sessionId, authToken = '') {
  const ws = useRef(null);
  const reconnectAttempts = useRef(0);
  const intentionalClose = useRef(false);
  const terminalSubs = useRef(new Set());
  const pendingTerminal = useRef([]);

  const [connected, setConnected] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [connectionLost, setConnectionLost] = useState(false);
  const [busy, setBusy] = useState(false);
  const [cwd, setCwd] = useState('~');
  const [startedAt, setStartedAt] = useState(null);
  const [restoredHistory, setRestoredHistory] = useState([]);

  const [health, setHealth] = useState('healthy');
  const [vizEvent, setVizEvent] = useState(null);
  const [samples, setSamples] = useState([]);
  const [latest, setLatest] = useState({ cpu: null, memory: null, pids: null, memoryBytes: null, memoryLimit: null });

  const [crash, setCrash] = useState(null);
  const [lastFailure, setLastFailure] = useState(null);
  const [timeline, setTimeline] = useState([]);

  const [aiText, setAiText] = useState('');
  const [aiSubject, setAiSubject] = useState(null);
  const [aiHistory, setAiHistory] = useState([]);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [aiFallback, setAiFallback] = useState(false);
  const aiTextRef = useRef('');
  const aiSubjectRef = useRef(null);

  const [lab, setLab] = useState(null);
  const [labVersion, setLabVersion] = useState(0);
  const [sessionSummary, setSessionSummary] = useState(null);
  const [lastError, setLastError] = useState(null);

  const emitTerminal = useCallback((event) => {
    if (terminalSubs.current.size === 0) {
      pendingTerminal.current.push(event);
      return;
    }
    terminalSubs.current.forEach((fn) => fn(event));
  }, []);

  const subscribeTerminal = useCallback((fn) => {
    terminalSubs.current.add(fn);
    const pending = pendingTerminal.current;
    pendingTerminal.current = [];
    pending.forEach((event) => fn(event));
    return () => terminalSubs.current.delete(fn);
  }, []);

  const pushTimeline = useCallback((entry) => {
    setTimeline((prev) => [...prev.slice(-(MAX_TIMELINE - 1)), { id: nextId(), at: Date.now(), ...entry }]);
  }, []);

  const finishAI = useCallback(() => {
    const text = aiTextRef.current;
    const subject = aiSubjectRef.current;
    if (text) {
      setAiHistory((h) => [...h.slice(-9), { id: nextId(), text, subject }]);
    }
  }, []);

  const handleMessage = useCallback(
    (raw) => {
      let msg;
      try {
        msg = JSON.parse(raw);
      } catch {
        return;
      }
      const type = msg.type;
      const payload = msg.payload ?? {};

      switch (type) {
        case 'output': {
          const text = (payload.stdout || '') + (payload.stderr || '');
          if (text) {
            emitTerminal({ type: 'output', text, notice: !!payload.notice });
          }
          if (payload.completed) {
            const exitCode = payload.exitCode ?? 0;
            if (payload.cwd) setCwd(payload.cwd);
            setBusy(false);
            emitTerminal({ type: 'complete', exitCode, cwd: payload.cwd });
            if (payload.command) {
              pushTimeline({
                kind: 'command',
                command: payload.command,
                exitCode,
                durationMs: payload.durationMs ?? null,
              });
              if (exitCode !== 0) {
                setLastFailure({ command: payload.command, exitCode });
              }
            }
          }
          break;
        }
        case 'shell':
          if (payload.cwd) setCwd(payload.cwd);
          if (payload.startedAt) setStartedAt(payload.startedAt);
          if (Array.isArray(payload.history)) setRestoredHistory(payload.history);
          break;
        case 'crash': {
          const info = {
            message: payload.data || 'Process crashed',
            exitCode: payload.exitCode,
            command: payload.command,
            event: payload.event,
            at: Date.now(),
          };
          setCrash(info);
          setLastFailure({ command: payload.command, exitCode: payload.exitCode });
          emitTerminal({ type: 'crash', ...info });
          pushTimeline({ kind: 'crash', command: payload.command, exitCode: payload.exitCode, message: info.message });
          break;
        }
        case 'visualization':
          if (payload.status) {
            setHealth(payload.status);
            if (payload.status === 'healthy') setVizEvent(null);
          }
          if (payload.event) setVizEvent(payload.event);
          if (payload.cpu != null || payload.memory != null) {
            const sample = {
              t: Date.now(),
              cpu: payload.cpu ?? 0,
              memory: payload.memory ?? 0,
              pids: payload.pids ?? null,
              traffic: payload.traffic ?? null,
            };
            setSamples((prev) => [...prev.slice(-(MAX_SAMPLES - 1)), sample]);
            setLatest({
              cpu: payload.cpu ?? null,
              memory: payload.memory ?? null,
              pids: payload.pids ?? null,
              memoryBytes: payload.memoryBytes ?? null,
              memoryLimit: payload.memoryLimit ?? null,
              traffic: payload.traffic ?? null,
            });
          }
          break;
        case 'ai_stream':
          if (payload.reset) {
            aiTextRef.current = '';
            setAiText('');
            setAiFallback(false);
            setIsAnalyzing(true);
            if (payload.subject) {
              aiSubjectRef.current = payload.subject;
              setAiSubject(payload.subject);
            }
          }
          if (payload.fallback) setAiFallback(true);
          if (payload.token) {
            aiTextRef.current += payload.token;
            setAiText(aiTextRef.current);
          }
          if (payload.done) {
            setIsAnalyzing(false);
            finishAI();
          } else if (!payload.reset && payload.token) {
            setIsAnalyzing(true);
          }
          break;
        case 'lab':
        case 'scenario':
          if (payload.id && payload.title) {
            setLab(payload);
            if (payload.state === 'completed') {
              setLabVersion((v) => v + 1);
              pushTimeline({ kind: 'lab', command: payload.title, message: 'Lab completed' });
            } else if (payload.state === 'preparing') {
              pushTimeline({ kind: 'lab', command: payload.title, message: 'Lab started' });
            }
          } else if (payload.active === false) {
            setLab(null);
          }
          break;
        case 'session_summary':
          setSessionSummary(payload);
          break;
        case 'error':
          setBusy(false);
          setLastError({ message: payload.message || 'error', at: Date.now() });
          emitTerminal({ type: 'error', text: payload.message || 'error' });
          break;
        default:
          break;
      }
    },
    [emitTerminal, pushTimeline, finishAI],
  );

  const connect = useCallback(() => {
    if (!sessionId) return;
    intentionalClose.current = false;
    setReconnecting(reconnectAttempts.current > 0);
    const params = new URLSearchParams({ session_id: sessionId });
    if (authToken) params.set('access_token', authToken);
    else params.set('guest', '1');
    const socket = new WebSocket(`${WS_URL}?${params.toString()}`);
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
        const delay = 1000 * 2 ** reconnectAttempts.current;
        setReconnecting(true);
        reconnectAttempts.current += 1;
        setTimeout(connect, delay);
      } else {
        setReconnecting(false);
        setConnectionLost(true);
      }
    };
    socket.onerror = () => {};
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

  const send = useCallback(
    (type, payload) => {
      if (ws.current?.readyState !== WebSocket.OPEN) return false;
      ws.current.send(JSON.stringify({ type, sessionId, payload }));
      return true;
    },
    [sessionId],
  );

  const sendCommand = useCallback(
    (command) => {
      if (inputDisabled) return false;
      setBusy(true);
      setHealth((h) => (h === 'analyzing' ? h : 'healthy'));
      setVizEvent(null);
      return send('command', { command, executionType: 'interactive' });
    },
    [inputDisabled, send],
  );

  const interrupt = useCallback(() => send('interrupt', {}), [send]);

  const labAction = useCallback(
    (action, id) => {
      if (action === 'start') {
        setCrash(null);
        setVizEvent(null);
      }
      return send('lab', { action, id });
    },
    [send],
  );

  const dismissLab = useCallback(() => setLab(null), []);

  const askAI = useCallback(
    (question, simpler = false) => {
      if (!send('ai_question', { question, simpler })) return;
      setIsAnalyzing(true);
    },
    [send],
  );

  const explainLastFailure = useCallback(() => {
    if (!send('ai_explain', {})) return;
    setIsAnalyzing(true);
  }, [send]);

  return {
    connected,
    reconnecting,
    connectionLost,
    inputDisabled,
    busy,
    cwd,
    startedAt,
    restoredHistory,
    health,
    vizEvent,
    samples,
    latest,
    crash,
    lastFailure,
    timeline,
    aiText,
    aiSubject,
    aiHistory,
    isAnalyzing,
    aiFallback,
    lab,
    labVersion,
    sessionSummary,
    lastError,
    subscribeTerminal,
    sendCommand,
    interrupt,
    labAction,
    dismissLab,
    askAI,
    explainLastFailure,
  };
}
