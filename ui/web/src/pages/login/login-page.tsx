import { useState } from "react";
import { useNavigate, useLocation } from "react-router";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@/stores/use-auth-store";
import { ROUTES } from "@/lib/constants";
import { useOidcAuth } from "@/hooks/use-oidc-auth";
import { LoginLayout } from "./login-layout";
import { LoginTabs, type LoginMode } from "./login-tabs";
import { TokenForm } from "./token-form";
import { PairingForm } from "./pairing-form";
import { OidcLoginButton } from "./oidc-login-button";

export function LoginPage() {
  const { t } = useTranslation("login");
  const [mode, setMode] = useState<LoginMode>("token");

  const setCredentials = useAuthStore((s) => s.setCredentials);
  const setPairing = useAuthStore((s) => s.setPairing);
  const { oidcEnabled, loginWithOidc } = useOidcAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const from =
    (location.state as { from?: { pathname: string } })?.from?.pathname ??
    ROUTES.OVERVIEW;

  function handleTokenLogin(userId: string, token: string) {
    setCredentials(token, userId);
    navigate(from, { replace: true });
  }

  function handlePairingApproved(senderID: string, userId: string) {
    setPairing(senderID, userId);
    setTimeout(() => navigate(from, { replace: true }), 500);
  }

  return (
    <LoginLayout subtitle={t("subtitle")}>
      <LoginTabs mode={mode} onModeChange={setMode} />
      {mode === "token" ? (
        <TokenForm onSubmit={handleTokenLogin} />
      ) : (
        <PairingForm onApproved={handlePairingApproved} />
      )}
      {oidcEnabled && (
        <OidcLoginButton onLogin={() => loginWithOidc(from)} />
      )}
    </LoginLayout>
  );
}
