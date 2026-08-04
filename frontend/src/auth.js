import Keycloak from 'keycloak-js';

const oidcUrl = import.meta.env.VITE_OIDC_URL || '';
const oidcRealm = import.meta.env.VITE_OIDC_REALM || '';
const oidcClientId = import.meta.env.VITE_OIDC_CLIENT_ID || '';

export const oidcConfigured = Boolean(oidcUrl && oidcRealm && oidcClientId);

let keycloak = null;
let initPromise = null;

function getKeycloak() {
  if (!keycloak) {
    keycloak = new Keycloak({
      url: oidcUrl,
      realm: oidcRealm,
      clientId: oidcClientId,
    });
  }
  return keycloak;
}

async function ensureInit() {
  if (!oidcConfigured) return;
  if (!initPromise) {
    initPromise = getKeycloak()
      .init({
        onLoad: 'check-sso',
        pkceMethod: 'S256',
        checkLoginIframe: false,
        silentCheckSsoRedirectUri: `${window.location.origin}/silent-check-sso.html`,
      })
      .catch(() => {});
  }
  return initPromise;
}

function currentTokenUser(kc) {
  const parsed = kc.tokenParsed || {};
  const id = parsed.sub || parsed.preferred_username || parsed.email || '';
  return {
    id,
    email: parsed.email || parsed.preferred_username || id,
    token: kc.token || '',
    provider: 'keycloak',
  };
}

export async function currentOIDCUser() {
  if (!oidcConfigured) return null;
  await ensureInit();
  const kc = getKeycloak();
  if (!kc.authenticated) return null;
  return currentTokenUser(kc);
}

export async function signInWithOIDC() {
  if (!oidcConfigured) throw new Error('OIDC is not configured for this deployment.');
  await ensureInit();
  const kc = getKeycloak();
  if (!kc.authenticated) {
    // Redirects to the Keycloak login page, then back to this app.
    kc.login({ redirectUri: window.location.href });
  }
}

export async function signOutFromOIDC() {
  if (!oidcConfigured) return;
  await ensureInit();
  const kc = getKeycloak();
  if (kc.authenticated) {
    await kc.logout({ redirectUri: window.location.origin });
  }
}
