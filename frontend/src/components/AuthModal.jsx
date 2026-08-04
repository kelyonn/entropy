import { useState } from 'react';
import { oidcConfigured, signInWithOIDC } from '../auth';

export default function AuthModal({ onAuthenticated, onClose }) {
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (event) => {
    event.preventDefault();
    setError('');
    setBusy(true);
    try {
      if (!oidcConfigured) {
        throw new Error('OIDC is not configured for this deployment.');
      }
      // keycloak-js performs a redirect-based login flow.
      await signInWithOIDC();
    } catch (err) {
      setError(err.message || 'Authentication failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4">
      <div className="panel w-full max-w-sm p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-ink">Sign in</h2>
            <p className="mt-1 text-xs text-ink-muted">
              You will be redirected to your OIDC provider (for example Keycloak).
            </p>
          </div>
          <button type="button" onClick={onClose} className="text-xs text-ink-muted">
            Close
          </button>
        </div>

        <form onSubmit={submit} className="mt-5 space-y-3">
          {error && <p className="text-xs text-danger">{error}</p>}
          <button
            type="submit"
            disabled={busy}
            className="w-full rounded-panel bg-accent px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
          >
            {busy ? 'Redirecting...' : 'Continue with OIDC'}
          </button>
        </form>
      </div>
    </div>
  );
}
