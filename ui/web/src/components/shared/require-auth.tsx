import { Navigate, useLocation } from "react-router";
import { useAuthStore } from "@/stores/use-auth-store";
import { ROUTES } from "@/lib/constants";

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token);
  const userId = useAuthStore((s) => s.userId);
  const senderID = useAuthStore((s) => s.senderID);
  const connected = useAuthStore((s) => s.connected);
  const tenantSelected = useAuthStore((s) => s.tenantSelected);
  const availableTenants = useAuthStore((s) => s.availableTenants);
  const isOwner = useAuthStore((s) => s.isOwner);
  const oidcEnabled = useAuthStore((s) => s.oidcEnabled);
  const oidcStatusLoaded = useAuthStore((s) => s.oidcStatusLoaded);
  const location = useLocation();

  // Wait for OIDC status to load before making auth decisions.
  // oidcEnabled starts false and is set async from /v1/auth/status.
  // Without this guard, we'd incorrectly apply token-mode logic (which requires userId)
  // before knowing we're in OIDC mode (which only requires token).
  if (!oidcStatusLoaded) {
    return null;
  }

  // Not authenticated
  // In OIDC mode, the JWT token alone is the credential — userId is populated async from /me.
  // In token mode, both token (or senderID) and userId are required.
  const notAuthenticated = oidcEnabled ? !token : (!token && !senderID) || !userId;
  if (notAuthenticated) {
    if (oidcEnabled) {
      // Redirect to Keycloak, setting /auth/callback as the post-login destination
      // so AuthCallbackPage can extract the token from the URL fragment.
      const callbackUrl = encodeURIComponent(
        window.location.origin + ROUTES.AUTH_CALLBACK,
      );
      window.location.href = `/v1/auth/login?redirect=${callbackUrl}`;
      return null;
    }
    // Fallback to local login page
    return <Navigate to={ROUTES.LOGIN} state={{ from: location }} replace />;
  }

  // Connected but no tenant selected — show tenant selector
  // (only gate after WS is connected and tenants have loaded)
  if (connected && !tenantSelected && availableTenants.length > 0) {
    return <Navigate to={ROUTES.SELECT_TENANT} state={{ from: location }} replace />;
  }

  // Connected, no tenants, not owner — blocked
  if (connected && !tenantSelected && availableTenants.length === 0 && !isOwner) {
    return <Navigate to={ROUTES.SELECT_TENANT} replace />;
  }

  return <>{children}</>;
}
