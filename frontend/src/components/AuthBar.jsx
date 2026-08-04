export default function AuthBar({ user, onGuest, onSignIn, onSignOut, authError, oidcConfigured }) {
  return (
    <div className="flex flex-col items-end gap-1">
      <div className="flex items-center gap-3 text-xs">
      {user ? (
        <>
          <span className="text-ink-muted">{user.email || user.id}</span>
          <button type="button" onClick={onSignOut} className="text-accent font-medium">
            Sign out
          </button>
        </>
      ) : (
        <>
          <button type="button" onClick={onGuest} className="text-ink-muted">
            Guest
          </button>
          {oidcConfigured && (
            <button type="button" onClick={onSignIn} className="text-accent font-medium">
              Sign in
            </button>
          )}
        </>
      )}
      </div>
      {authError && <span className="text-xs text-danger">{authError}</span>}
    </div>
  );
}
