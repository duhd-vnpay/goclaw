import { useEffect, useRef } from "react";
import { useNavigate } from "react-router";
import { useOidcAuth } from "@/hooks/use-oidc-auth";
import { ROUTES } from "@/lib/constants";

/**
 * AuthCallbackPage handles the redirect from Keycloak.
 * Extracts the access_token from URL fragment and stores it.
 */
export function AuthCallbackPage() {
  const navigate = useNavigate();
  const { handleCallbackToken } = useOidcAuth();
  const processedRef = useRef(false);

  useEffect(() => {
    if (processedRef.current) return;
    processedRef.current = true;

    // Extract access_token from URL fragment (#access_token=...)
    const hash = window.location.hash.substring(1);
    const params = new URLSearchParams(hash);
    const accessToken = params.get("access_token");

    if (accessToken) {
      handleCallbackToken(accessToken);
      // Clean up the URL fragment
      window.history.replaceState(null, "", window.location.pathname);
      // Navigate to overview after a brief delay for state to settle
      setTimeout(() => navigate(ROUTES.OVERVIEW, { replace: true }), 100);
    } else {
      // No token in fragment -- might be a direct /v1/auth/callback response
      // Check if the backend set a cookie (non-SPA flow)
      fetch("/v1/auth/me", { credentials: "include" })
        .then((res) => {
          if (res.ok) return res.json();
          throw new Error("not authenticated");
        })
        .then(() => {
          // Backend cookie auth -- store user info
          handleCallbackToken("cookie"); // sentinel value
          navigate(ROUTES.OVERVIEW, { replace: true });
        })
        .catch(() => {
          // Failed -- redirect to login
          navigate(ROUTES.LOGIN, { replace: true });
        });
    }
  }, [navigate, handleCallbackToken]);

  return (
    <div className="flex h-dvh items-center justify-center">
      <div className="flex flex-col items-center gap-4">
        <img
          src="/goclaw-icon.svg"
          alt=""
          className="h-12 w-12 animate-pulse"
        />
        <p className="text-sm text-muted-foreground">
          Completing sign-in...
        </p>
      </div>
    </div>
  );
}
