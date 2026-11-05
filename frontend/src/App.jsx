import { useState, useEffect, useCallback, useRef } from 'react';
import axios from 'axios';
import Terminal from './components/Terminal';
import AIPanel from './components/AIPanel';
import SystemView from './components/SystemView';
import LabPanel from './components/LabPanel';
import Timeline from './components/Timeline';
import Onboarding from './components/Onboarding';
import AuthBar from './components/AuthBar';
import AuthModal from './components/AuthModal';
import SessionHistory from './components/SessionHistory';
import { useWebSocket } from './hooks/useWebSocket';
import { currentOIDCUser, signOutFromOIDC, oidcConfigured } from './auth';

const API_URL = import.meta.env.VITE_API_URL || '';
const ONBOARD_KEY = 'entropy_onboarded_v1';
const SESSION_LIMIT_SECONDS = 60 * 60;
const WS_BASE =
  import.meta.env.VITE_WS_URL || `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/ws`;

function safeGet(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function safeSet(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* storage unavailable */
  }
}

function useClock(startedAt) {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  if (!startedAt) return null;
  return Math.max(0, Math.floor((now - new Date(startedAt).getTime()) / 1000));
}

function clock(seconds) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

function ConnectionStatus({ connected, reconnecting, connectionLost }) {
  let label = 'Offline';
  let color = 'text-ink-muted';
  let dot = 'bg-ink-muted';
  if (connectionLost) {
    label = 'Connection lost';
    color = 'text-danger';
    dot = 'bg-danger';
  } else if (reconnecting) {
    label = 'Reconnecting';
    color = 'text-warning';
    dot = 'bg-warning';
  } else if (connected) {
    label = 'Connected';
    color = 'text-success';
    dot = 'bg-success';
  }
  return (
    <span className={`flex items-center gap-1.5 status-fade font-medium ${color}`}>
      <span className={`h-1.5 w-1.5 rounded-[2px] ${dot}`} aria-hidden="true" />
      {label}
    </span>
  );
}

export default function App() {
  const [sessionId, setSessionId] = useState(null);
  const [bootError, setBootError] = useState('');
  const [showOnboarding, setShowOnboarding] = useState(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get('session_id')) return false;
    return safeGet(ONBOARD_KEY) !== '1';
  });
  const [drawerOpen, setDrawerOpen] = useState(true);
  const [toast, setToast] = useState(null);
  const [user, setUser] = useState(null);
  const [history, setHistory] = useState([]);
  const [authError, setAuthError] = useState('');
  const [authOpen, setAuthOpen] = useState(false);
  const [snapshotSaved, setSnapshotSaved] = useState(false);
  const queueTimer = useRef(null);
  const toastTimer = useRef(null);

  const showToast = useCallback((message, tone = 'ok') => {
    clearTimeout(toastTimer.current);
    setToast({ message, tone });
    toastTimer.current = setTimeout(() => setToast(null), tone === 'ok' ? 3000 : 5000);
  }, []);

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
        .catch(() => setUser(null));
    };
    refresh();
    const timer = setInterval(refresh, 5 * 60 * 1000);
    return () => clearInterval(timer);
  }, []);

  const startSession = useCallback(
    async (requestedToken = '') => {
      const queueToken = typeof requestedToken === 'string' ? requestedToken : '';
      setBootError('');
      try {
        const res = await axios.get(`${API_URL}/api/session/new`, {
          headers: authHeaders(),
          params: queueToken ? { queue_token: queueToken } : {},
        });
        if (res.data.queued) {
          setBootError(
            `All sandboxes are busy. You are number ${res.data.position} in the queue, about ${res.data.estimated_wait_sec}s.`,
          );
          clearTimeout(queueTimer.current);
          queueTimer.current = setTimeout(() => startSession(res.data.queue_token), 3000);
          return;
        }
        clearTimeout(queueTimer.current);
        const id = res.data.session_id;
        setSessionId(id);
        window.history.pushState({}, '', `?session_id=${id}`);
        safeSet(ONBOARD_KEY, '1');
        setShowOnboarding(false);
      } catch (err) {
        setBootError(
          err.response?.status === 429 ? 'Session limit reached. Try again shortly.' : err.message || 'Failed to create session',
        );
      }
    },
    [authHeaders],
  );

  useEffect(() => () => clearTimeout(queueTimer.current), []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const existing = params.get('session_id');
    if (existing) {
      setSessionId(existing);
      setShowOnboarding(false);
      return;
    }
    if (!showOnboarding) startSession();
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

  const wsState = useWebSocket(sessionId, user?.token || '');
  const {
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
  } = wsState;

  const elapsed = useClock(startedAt);

  // The interactive terminal attaches once the sandbox reports ready (the
  // shell message arrives after the container is up), because the control
  // channel owns the sandbox.
  const [ptyUrl, setPtyUrl] = useState(null);
  useEffect(() => {
    if (!connected || !startedAt || !sessionId || ptyUrl) return;
    const params = new URLSearchParams({ session_id: sessionId });
    if (user?.token) params.set('access_token', user.token);
    else params.set('guest', '1');
    setPtyUrl(`${WS_BASE}/pty?${params.toString()}`);
  }, [connected, startedAt, sessionId, user, ptyUrl]);

  useEffect(() => {
    if (lastError) showToast(lastError.message, 'warn');
  }, [lastError, showToast]);

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
      await axios.post(`${API_URL}/api/session/snapshot?session_id=${sessionId}`, null, { headers: authHeaders() });
      setSnapshotSaved(true);
      showToast('Snapshot saved. Filesystem and history can be restored.');
    } catch (err) {
      showToast(err.response?.data || err.message || 'Snapshot failed', 'warn');
    }
  };

  const handleRestore = async () => {
    try {
      const res = await axios.get(`${API_URL}/api/session/restore?session_id=${sessionId}`, { headers: authHeaders() });
      window.location.href = `/?session_id=${res.data.session_id}`;
    } catch (err) {
      showToast(`Restore failed: ${err.response?.data || err.message}`, 'warn');
    }
  };

  const askAndOpen = useCallback(
    (q, simpler) => {
      setDrawerOpen(true);
      askAI(q, simpler);
    },
    [askAI],
  );

  const explainAndOpen = useCallback(() => {
    setDrawerOpen(true);
    explainLastFailure();
  }, [explainLastFailure]);

  if (showOnboarding) {
    return <Onboarding onStart={startSession} error={bootError} />;
  }

  if (!sessionId) {
    return (
      <div className="min-h-screen bg-canvas flex items-center justify-center p-6">
        <div className="panel px-6 py-5 max-w-sm text-center">
          <div className="text-sm font-semibold text-ink">{bootError ? 'Waiting for a sandbox' : 'Preparing sandbox'}</div>
          <p className="mt-1 text-sm text-ink-muted status-fade">{bootError || 'Starting an isolated container.'}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-canvas text-ink">
      <header className="bg-surface border-b border-line">
        <div className="max-w-[1600px] mx-auto px-4 md:px-6 py-3 flex flex-wrap items-center justify-between gap-x-6 gap-y-2">
          <div className="flex items-baseline gap-3">
            <h1 className="text-xl font-bold tracking-tight text-accent">ENTROPY</h1>
            <span className="text-sm text-ink-muted hidden sm:inline">break it, watch it, understand it</span>
          </div>
          <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-xs">
            <ConnectionStatus connected={connected} reconnecting={reconnecting} connectionLost={connectionLost} />
            <span className="font-mono text-ink-muted" title={sessionId}>
              session {sessionId.slice(0, 8)}
            </span>
            {elapsed != null && (
              <span
                className={`tabular-nums ${elapsed > SESSION_LIMIT_SECONDS - 300 ? 'text-warning' : 'text-ink-muted'}`}
                title="Sessions end after one hour, or five minutes idle"
              >
                {clock(elapsed)} of 60:00
              </span>
            )}
            <span className="h-4 w-px bg-line hidden md:block" aria-hidden="true" />
            <button type="button" onClick={handleSnapshot} className="font-medium text-accent hover:underline">
              Save snapshot
            </button>
            {snapshotSaved && (
              <button type="button" onClick={handleRestore} className="font-medium text-accent hover:underline">
                Restore snapshot
              </button>
            )}
            <AuthBar
              user={user}
              onGuest={() => handleSignOut()}
              onSignIn={() => {
                setAuthError('');
                setAuthOpen(true);
              }}
              onSignOut={handleSignOut}
              authError={authError}
              oidcConfigured={oidcConfigured}
            />
          </div>
        </div>
      </header>

      {(reconnecting || connectionLost) && (
        <div className="bg-muted border-b border-line">
          <div className="max-w-[1600px] mx-auto px-4 md:px-6 py-2 text-sm text-warning">
            {connectionLost
              ? 'Connection lost. Refresh the page to reconnect to this sandbox.'
              : 'Reconnecting. Terminal input is paused.'}
          </div>
        </div>
      )}

      <main className="max-w-[1600px] mx-auto px-4 md:px-6 py-4 grid grid-cols-1 lg:grid-cols-12 gap-4">
        <div className="lg:col-span-7 xl:col-span-8 flex flex-col gap-4 min-w-0">
          <div className="h-[60vh] min-h-[380px]">
            <Terminal
              subscribe={subscribeTerminal}
              onCommand={sendCommand}
              onInterrupt={interrupt}
              enabled={!inputDisabled}
              busy={busy}
              cwd={cwd}
              restoredHistory={restoredHistory}
              ptyUrl={ptyUrl}
            />
          </div>
          <div className="max-h-56 flex flex-col">
            <Timeline entries={timeline} onExplain={explainAndOpen} />
          </div>
        </div>

        <aside className="lg:col-span-5 xl:col-span-4 flex flex-col gap-4 min-w-0">
          <div className="flex flex-col max-h-[60vh] min-h-[320px]">
            <LabPanel
              sessionId={sessionId}
              authHeaders={authHeaders}
              lab={lab}
              labVersion={labVersion}
              onAction={labAction}
              onDismiss={dismissLab}
              onAskAI={(q) => askAndOpen(q, false)}
              disabled={inputDisabled}
            />
          </div>
          <SystemView health={health} event={vizEvent} samples={samples} latest={latest} />
          {user && (
            <SessionHistory
              sessions={history}
              onRestore={(id) => {
                window.location.href = `/?session_id=${id}`;
              }}
            />
          )}
        </aside>

        <div className="lg:col-span-12">
          <AIPanel
            text={aiText}
            subject={aiSubject}
            history={aiHistory}
            isAnalyzing={isAnalyzing}
            aiFallback={aiFallback}
            lastFailure={lastFailure}
            connected={!inputDisabled}
            expanded={drawerOpen}
            onToggle={() => setDrawerOpen((v) => !v)}
            onAsk={askAndOpen}
            onExplainFailure={explainAndOpen}
          />
        </div>
      </main>

      {toast && (
        <div
          role="status"
          className={`fixed bottom-4 right-4 z-40 panel px-4 py-2.5 text-sm max-w-sm ${
            toast.tone === 'ok' ? 'text-success' : 'text-warning'
          }`}
        >
          {toast.message}
        </div>
      )}

      {sessionSummary && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-[#14181f]/30 p-4">
          <div className="panel max-w-md w-full p-6" role="dialog" aria-modal="true" aria-labelledby="summary-title">
            <h3 id="summary-title" className="text-lg font-semibold text-ink">
              Session ended
            </h3>
            <p className="mt-1 text-sm text-ink-muted">{sessionSummary.reason || 'Session closed'}</p>
            <dl className="mt-5 grid grid-cols-2 gap-px bg-line border border-line rounded-panel overflow-hidden text-sm">
              {[
                ['Commands', sessionSummary.commandsRun ?? 0],
                ['Crashes', sessionSummary.crashesEncountered ?? 0],
                ['Labs resolved', sessionSummary.labsCompleted ?? 0],
                ['Duration', clock(sessionSummary.durationSeconds ?? 0)],
              ].map(([k, v]) => (
                <div key={k} className="bg-surface px-3 py-2.5">
                  <dt className="text-xs text-ink-muted">{k}</dt>
                  <dd className="text-lg font-semibold tabular-nums text-ink">{v}</dd>
                </div>
              ))}
            </dl>
            <button
              type="button"
              onClick={() => {
                window.location.href = '/';
              }}
              className="mt-6 w-full bg-accent text-white py-2 rounded-panel text-sm font-medium hover:opacity-90"
            >
              Start a new session
            </button>
          </div>
        </div>
      )}

      {authOpen && <AuthModal onClose={() => setAuthOpen(false)} />}
    </div>
  );
}
