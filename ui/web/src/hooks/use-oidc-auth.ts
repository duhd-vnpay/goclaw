import { useCallback, useEffect, useRef } from "react";
import { useAuthStore, type OidcUserProfile } from "@/stores/use-auth-store";

interface AuthStatusResponse {
  keycloak_enabled: boolean;
  login_url: string;
  realm_url?: string;
  client_id?: string;
}

interface MeResponse extends OidcUserProfile {
  status: string;
  last_login_at: string | null;
}

/**
 * useOidcAuth checks if Keycloak OIDC is enabled and manages the OIDC session.
 * Call once at app root (e.g. in RequireAuth or App).
 */
export function useOidcAuth() {
  const token = useAuthStore((s) => s.token);
  const oidcEnabled = useAuthStore((s) => s.oidcEnabled);
  const oidcUser = useAuthStore((s) => s.oidcUser);
  const setOidcEnabled = useAuthStore((s) => s.setOidcEnabled);
  const setOidcUser = useAuthStore((s) => s.setOidcUser);
  const setCredentials = useAuthStore((s) => s.setCredentials);
  const checkedRef = useRef(false);

  // Check OIDC status on mount (once)
  useEffect(() => {
    if (checkedRef.current) return;
    checkedRef.current = true;

    fetch("/v1/auth/status")
      .then((res) => res.json())
      .then((data: AuthStatusResponse) => {
        setOidcEnabled(data.keycloak_enabled);
      })
      .catch(() => {
        // Auth status endpoint not available -- Keycloak not configured
        setOidcEnabled(false);
      });
  }, [setOidcEnabled]);

  // Fetch /me when we have a token but no oidcUser
  useEffect(() => {
    if (!token || oidcUser) return;

    fetch("/v1/auth/me", {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((res) => {
        if (!res.ok) return null;
        return res.json();
      })
      .then((data: MeResponse | null) => {
        if (data) {
          setOidcUser({
            id: data.id,
            email: data.email,
            display_name: data.display_name,
            avatar_url: data.avatar_url,
            auth_provider: data.auth_provider,
            role: data.role,
            realm_roles: data.realm_roles,
            groups: data.groups,
          });
        }
      })
      .catch(() => {
        // /me failed -- token might be invalid
      });
  }, [token, oidcUser, setOidcUser]);

  const loginWithOidc = useCallback(
    (redirectPath?: string) => {
      const loginUrl = redirectPath
        ? `/v1/auth/login?redirect=${encodeURIComponent(redirectPath)}`
        : "/v1/auth/login";
      window.location.href = loginUrl;
    },
    [],
  );

  const handleCallbackToken = useCallback(
    (accessToken: string) => {
      setCredentials(accessToken, "");
      // /me will be fetched by the useEffect above
    },
    [setCredentials],
  );

  const logoutOidc = useCallback(async () => {
    try {
      const res = await fetch("/v1/auth/logout", { method: "POST" });
      const data = await res.json();
      if (data.logout_url) {
        // Redirect to Keycloak logout, then back to our login page
        const postLogoutRedirect = `${window.location.origin}/login`;
        window.location.href = `${data.logout_url}?post_logout_redirect_uri=${encodeURIComponent(postLogoutRedirect)}&client_id=goclaw-gateway`;
        return;
      }
    } catch {
      // Fallback: just clear local state
    }
    useAuthStore.getState().logout();
  }, []);

  return {
    oidcEnabled,
    oidcUser,
    loginWithOidc,
    handleCallbackToken,
    logoutOidc,
    isAuthenticated: !!token && !!oidcUser,
  };
}
