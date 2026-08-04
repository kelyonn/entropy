import { useState, useEffect, useCallback, useRef } from 'react';
import Terminal from './components/Terminal';
import AIPanel from './components/AIPanel';
import ServerViz from './components/ServerViz';
import Dashboard from './components/Dashboard';
import StatusBar from './components/StatusBar';
import Onboarding from './components/Onboarding';
import Scenarios from './components/Scenarios';
import AuthBar from './components/AuthBar';
import AuthModal from './components/AuthModal';
import CommandFlow from './components/CommandFlow';
import SessionHistory from './components/SessionHistory';
import { useWebSocket } from './hooks/useWebSocket';
import { currentOIDCUser, signOutFromOIDC, oidcConfigured } from './auth';
import axios from 'axios';

const API_URL = import.meta.env.VITE_API_URL || '';
const ONBOARD_KEY = 'entropy_onboarded_v1';

export default function App() {
  const [sessionId, setSessionId] = useState(null);
  const [bootError, setBootError] = useState('');
  const [showOnboarding, setShowOnboarding] = useState(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get('session_id')) return false;
    return localStorage.getItem(ONBOARD_KEY) !== '1';
  });
  const [drawerOpen, setDrawerOpen] = useState(true);
  const [toast, setToast] = useState('');
  const [user, setUser] = useState(null);
  const [history, setHistory] = useState([]);
  const [authError, setAuthError] = useState('');
  const [authOpen, setAuthOpen] = useState(false);
  const queueTimer = useRef(null);

  const authHeaders = useCallback(() => {
    const h = {};
    if (user?.token) {
      h.Authorization = `Bearer ${user.token}`;
      h['X-User-Id'] = user.id;
    } else if (user?.id) {
      h['X-User-Id'] = user.id;
    } else {
      h['X-Guest'] = '1';
    }
    return h;
  }, [user]);

  useEffect(() => {
    const refresh = () => {
      currentOIDCUser()
        .then((nextUser) => {
          if (nextUser) setUser(nextUser);
        })
        .catch(() => {
          setUser(null);
        });
    };
    refresh();
    const timer = setInterval(refresh, 5 * 60 * 1000);
    return () => clearInterval(timer);
  }, []);

  const startSession = useCallback(async (requestedToken = '') => {
    const queueToken = typeof requestedToken === 'string' ? requestedToken : '';
    setBootError('');
    try {
      const res = await axios.get(`${API_URL}/api/session/new`, {
        headers: authHeaders(),
        params: queueToken ? { queue_token: queueToken } : {},
      });
      if (res.data.queued) {
        setBootError(
          `All sandboxes busy. Queue position ${res.data.position}. Estimated wait ${res.data.estimated_wait_sec}s.`,
        );
        clearTimeout(queueTimer.current);
        queueTimer.current = setTimeout(() => startSession(res.data.queue_token), 3000);
        return;
      }
      clearTimeout(queueTimer.current);
      const id = res.data.session_id;
      setSessionId(id);
      window.history.pushState({}, '', `?session_id=${id}`);
      localStorage.setItem(ONBOARD_KEY, '1');
      setShowOnboarding(false);
    } catch (err) {
      const msg =
        err.response?.status === 429
          ? 'Session limit reached. Try again shortly.'
          : err.message || 'Failed to create session';
      setBootError(msg);
    }
  }, [user, authHeaders]);

  useEffect(() => () => clearTimeout(queueTimer.current), []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const existing = params.get('session_id');
    if (existing) {
      setSessionId(existing);
      setShowOnboarding(false);
      return;
    }
    if (!showOnboarding) {
      startSession();
    }
  }, [showOnboarding, startSession]);

  useEffect(() => {
    if (!user?.id) {
      setHistory([]);
      return;
    }
    axios
      .get(`${API_URL}/api/sessions`, { headers: authHeaders() })
      .then((res) => setHistory(res.data.sessions || []))
      .catch(() => setHistory([]));
  }, [user, sessionId, authHeaders]);

  const {
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
  } = useWebSocket(sessionId, user?.token || '');

  const handleSignIn = () => {
    setAuthError('');
    setAuthOpen(true);
  };

  const handleSignOut = async () => {
    try {
      await signOutFromOIDC();
      setUser(null);
    } catch (err) {
      setAuthError(err.message || 'Sign-out failed');
    }
  };

  const handleSnapshot = async () => {
    try {
      await axios.post(`${API_URL}/api/session/snapshot?session_id=${sessionId}`, null, {
        headers: authHeaders(),
      });
      setToast('Snapshot saved');
      setTimeout(() => setToast(''), 3000);
    } catch (err) {
      setToast(err.response?.data || err.message || 'Snapshot failed');
      setTimeout(() => setToast(''), 4000);
    }
  };

  if (showOnboarding) {
    return <Onboarding onStart={startSession} error={bootError} />;
  }

  if (!sessionId) {
    return (
      <div className="min-h-screen bg-canvas flex items-center justify-center">
        <div className="text-ink text-lg status-fade">
          {bootError || 'Preparing sandbox...'}
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-canvas text-ink p-4 md:p-5">
      <header className="flex flex-wrap items-end justify-between gap-3 mb-4">
        <div>
          <h1 className="text-3xl font-bold tracking-tight text-accent">ENTROPY</h1>
          <p className="text-sm text-ink-muted mt-0.5">crash and learn</p>
        </div>
        <div className="flex flex-col items-end gap-2">
          <AuthBar
            user={user}
            onGuest={() => handleSignOut()}
            onSignIn={handleSignIn}
            onSignOut={handleSignOut}
            authError={authError}
            oidcConfigured={oidcConfigured}
          />
          <StatusBar
            connected={connected}
            reconnecting={reconnecting}
            connectionLost={connectionLost}
            sessionId={sessionId}
          />
          <button
            type="button"
            onClick={handleSnapshot}
            className="text-xs font-medium text-accent"
          >
            Save snapshot
          </button>
        </div>
      </header>

      {(reconnecting || connectionLost) && (
        <div className="mb-3 panel px-3 py-2 text-sm text-warning">
          {connectionLost
            ? 'Connection lost. Refresh the page to continue.'
            : 'Reconnecting... terminal input disabled.'}
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-5 gap-4 min-h-[calc(100vh-220px)]">
        <div className="lg:col-span-3 min-h-[420px] flex flex-col">
          <CommandFlow
            busy={busy}
            exitCode={exitCode}
            isAnalyzing={isAnalyzing}
            crashData={crashData}
          />
          <div className="flex-1 min-h-0">
          <Terminal
            sendCommand={sendCommand}
            outputChunk={outputChunk}
            exitCode={exitCode}
            crashMessage={crashData}
            connected={connected && !inputDisabled}
            busy={busy}
          />
          </div>
        </div>

        <div className="lg:col-span-2 flex flex-col gap-3">
          <ServerViz
            health={isAnalyzing ? 'analyzing' : health}
            event={vizEvent}
            cpu={resourceStats.cpu}
            memory={resourceStats.memory}
          />
          <Dashboard />
          <Scenarios
            onRun={runScenario}
            onInterrupt={interruptScenario}
            activeId={activeScenario}
            disabled={inputDisabled || busy}
          />
          {user && (
            <SessionHistory
              sessions={history}
              onRestore={(id) => {
                window.location.href = `/?session_id=${id}`;
              }}
            />
          )}
        </div>
      </div>

      <div className="mt-4">
        <AIPanel
          explanation={aiExplanation}
          history={aiHistory}
          isAnalyzing={isAnalyzing}
          crashData={crashData}
          sessionId={sessionId}
          expanded={drawerOpen}
          onToggle={() => setDrawerOpen((v) => !v)}
          toast={toast}
          onAskAI={askAI}
          aiFallback={aiFallback}
        />
      </div>

      {sessionSummary && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4">
          <div className="panel max-w-md w-full p-6">
            <h3 className="text-lg font-semibold text-ink">Session ended</h3>
            <p className="mt-2 text-sm text-ink-muted">{sessionSummary.reason || 'Session closed'}</p>
            <dl className="mt-4 grid grid-cols-2 gap-2 text-sm">
              <dt className="text-ink-muted">Commands</dt>
              <dd>{sessionSummary.commandsRun ?? 0}</dd>
              <dt className="text-ink-muted">Crashes</dt>
              <dd>{sessionSummary.crashesEncountered ?? 0}</dd>
              <dt className="text-ink-muted">Duration</dt>
              <dd>{sessionSummary.durationSeconds ?? 0}s</dd>
            </dl>
            <button
              type="button"
              onClick={() => window.location.reload()}
              className="mt-6 w-full bg-accent text-white py-2 rounded-panel text-sm font-medium"
            >
              Start new session
            </button>
          </div>
        </div>
      )}

      {authOpen && (
        <AuthModal
          onClose={() => setAuthOpen(false)}
        />
      )}
    </div>
  );
}
