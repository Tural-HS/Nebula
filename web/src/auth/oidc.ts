import { InMemoryWebStorage, UserManager, WebStorageStateStore } from "oidc-client-ts";

import type { AppConfig } from "../config";
import { activeLocale } from "../i18n/locale";

/*
 * OIDC Authorization Code flow with PKCE against the identity provider.
 *
 * Tokens live only in memory (InMemoryWebStorage): an XSS bug cannot read them
 * from localStorage/sessionStorage. A page reload performs a silent redirect
 * through the IdP, which still has its own SSO session, so users rarely notice.
 */

export const CALLBACK_PATH = "/auth/callback";

export function createUserManager(cfg: AppConfig): UserManager {
  const origin = window.location.origin;
  return new UserManager({
    authority: cfg.oidcAuthority,
    client_id: cfg.oidcClientId,
    redirect_uri: origin + CALLBACK_PATH,
    post_logout_redirect_uri: origin + "/",
    response_type: "code",
    scope: "openid profile email",
    userStore: new WebStorageStateStore({ store: new InMemoryWebStorage() }),
    automaticSilentRenew: true, // uses the refresh token before the access token expires
    monitorSession: false,
    ui_locales: activeLocale(), // the login page follows the UI language (realm: internationalization on)
  });
}

// Only same-origin relative paths are accepted as post-login destinations
// (prevents open redirects through the OIDC state parameter).
export function safeReturnPath(p: unknown): string {
  return typeof p === "string" && p.startsWith("/") && !p.startsWith("//") && !p.startsWith(CALLBACK_PATH) ? p : "/";
}
