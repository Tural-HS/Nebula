import type { User, UserManager } from "oidc-client-ts";
import { useEffect, useState, type ReactNode } from "react";

import { setTokenProvider } from "../api/client";
import { AuthContext, type AuthState } from "./context";
import { CALLBACK_PATH, safeReturnPath } from "./oidc";

// The authorization code is single-use. Effects can run twice (React StrictMode,
// remounts), so the callback is processed exactly once per page load and every
// caller shares the same result.
let callbackOnce: Promise<User> | null = null;

function handleCallbackOnce(manager: UserManager): Promise<User> {
  callbackOnce ??= manager.signinRedirectCallback();
  return callbackOnce;
}

function onCallbackPage(): boolean {
  return window.location.pathname === CALLBACK_PATH;
}

export function AuthProvider({ manager, children }: { manager: UserManager; children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [ready, setReady] = useState(false);
  const [signingOut, setSigningOut] = useState(false);

  useEffect(() => {
    const run = { cancelled: false };
    setTokenProvider(async () => {
      const u = await manager.getUser();
      return u && !u.expired ? u.access_token : null;
    });
    const onLoaded = (u: User) => { setUser(u); };
    const onGone = () => { setUser(null); };
    manager.events.addUserLoaded(onLoaded);
    manager.events.addUserUnloaded(onGone);
    manager.events.addSilentRenewError(onGone);

    void (async () => {
      try {
        if (onCallbackPage()) {
          const u = await handleCallbackOnce(manager);
          // The first of two concurrent runs may already have moved on.
          if (onCallbackPage()) {
            window.history.replaceState(null, "", safeReturnPath(u.state));
          }
          if (!run.cancelled) setUser(u);
        } else {
          const u = await manager.getUser();
          if (!run.cancelled) setUser(u && !u.expired ? u : null);
        }
      } catch (err) {
        console.error("sign-in failed", err);
        window.history.replaceState(null, "", "/");
      } finally {
        if (!run.cancelled) setReady(true);
      }
    })();

    return () => {
      run.cancelled = true;
      manager.events.removeUserLoaded(onLoaded);
      manager.events.removeUserUnloaded(onGone);
      manager.events.removeSilentRenewError(onGone);
    };
  }, [manager]);

  const state: AuthState = {
    user,
    signingOut,
    login: (returnTo) => manager.signinRedirect({ state: safeReturnPath(returnTo ?? window.location.pathname + window.location.search) }),
    logout: async () => {
      // signoutRedirect clears the in-memory user before the browser leaves the
      // page. Without this flag, "no user" would trigger an automatic login
      // redirect that cancels the IdP logout and silently signs the user back in.
      setSigningOut(true);
      try {
        await manager.signoutRedirect();
      } catch (err) {
        setSigningOut(false);
        throw err;
      }
    },
  };

  if (!ready) return null;
  return <AuthContext value={state}>{children}</AuthContext>;
}
