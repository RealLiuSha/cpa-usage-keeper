import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import './index.css';
import './App.css';
import './embed/cpamcEmbed.css';
import { ApiError, appPath, clearEmbedSessionToken, getSession, login, loginWithCPAAPIKey } from './lib/api';
import type { AuthRole, AuthSessionAPIKeySummary } from './lib/types';
import {
  isSharePublicFeatureEnabled,
  prepareShareModeFromLocation,
  resolveShareTab,
  stripAppBasePath,
  type ShareTab,
} from './lib/shareMode';
import { AppFooter } from './components/AppFooter';
import { KeyOverviewPage } from './pages/KeyOverviewPage';
import { LoginPage } from './pages/LoginPage';
import { UsagePage } from './pages/UsagePage';
import { cpamcEmbedSearch, isCPAMCEmbed, notifyCPAMCEmbedReady } from './embed/cpamcEmbed';
import { useUsageStatsStore } from './stores/useUsageStatsStore';

type AuthState = 'checking' | 'authenticated' | 'unauthenticated';

export const getRoleHomePath = (role: AuthRole): '/' | '/key-overview' => (
  role === 'api_key_viewer' ? '/key-overview' : '/'
);

export const shouldNormalizeRolePath = (
  role: AuthRole,
  currentPath: string,
  sharePublicEnabled: boolean = isSharePublicFeatureEnabled(),
): boolean => {
  // Only skip normalization when share public is actually enabled for this deployment.
  // Path-only checks leave admins stuck on /share/* after login when the feature is off.
  if (sharePublicEnabled && resolveShareTab(currentPath)) return false;
  return currentPath !== getRoleHomePath(role);
};

function App() {
  const { t } = useTranslation();
  // Pure path + server feature flag; apiPath re-derives share routing from location (no mutable global).
  const [shareTab] = useState<ShareTab | null>(() => (
    prepareShareModeFromLocation(window.location.pathname, window.__APP_BASE_PATH__)
  ));
  const isShareMode = Boolean(shareTab);
  const [authState, setAuthState] = useState<AuthState>(isShareMode ? 'unauthenticated' : 'checking');
  const [authRole, setAuthRole] = useState<AuthRole | null>(null);
  const [sessionAPIKey, setSessionAPIKey] = useState<AuthSessionAPIKeySummary | undefined>();
  const [adminLoginError, setAdminLoginError] = useState('');
  const [apiKeyLoginError, setAPIKeyLoginError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const clearUsageStats = useUsageStatsStore((state) => state.clearUsageStats);
  const isEmbeddedInCPAMC = isCPAMCEmbed();

  const clearSession = useCallback(() => {
    clearEmbedSessionToken();
    clearUsageStats();
    setAuthState('unauthenticated');
    setAuthRole(null);
    setSessionAPIKey(undefined);
  }, [clearUsageStats]);

  const applySession = useCallback((session: Awaited<ReturnType<typeof getSession>>) => {
    if (!session.authenticated) {
      clearSession();
      return;
    }
    setAuthState('authenticated');
    setAuthRole(session.role ?? 'admin');
    setSessionAPIKey(session.api_key);
  }, [clearSession]);

  const loadSession = useCallback(async () => {
    const session = await getSession();
    applySession(session);
    return session;
  }, [applySession]);

  useEffect(() => {
    // Share pages do not require a session; skip the login gate entirely.
    if (isShareMode) return;
    void loadSession().catch(() => {
      clearSession();
    });
  }, [clearSession, isShareMode, loadSession]);

  useEffect(() => {
    notifyCPAMCEmbedReady();
  }, []);

  useEffect(() => {
    if (authState !== 'authenticated' || !authRole) return;
    const currentPath = stripAppBasePath(window.location.pathname, window.__APP_BASE_PATH__);
    if (!shouldNormalizeRolePath(authRole, currentPath)) return;
    window.history.replaceState(null, '', appPath(getRoleHomePath(authRole)) + cpamcEmbedSearch());
  }, [authRole, authState]);

  const handlePasswordLogin = useCallback(async (password: string) => {
    setSubmitting(true);
    setAdminLoginError('');
    try {
      await login(password);
      const session = await loadSession();
      if (!session.authenticated) {
        setAdminLoginError(t('auth.login_failed'));
        clearSession();
        return;
      }
      window.history.replaceState(null, '', appPath('/') + cpamcEmbedSearch());
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        setAdminLoginError(t('auth.invalid_password'));
      } else {
        setAdminLoginError(t('auth.login_failed'));
      }
      clearSession();
    } finally {
      setSubmitting(false);
    }
  }, [clearSession, loadSession, t]);

  const handleAPIKeyLogin = useCallback(async (apiKey: string) => {
    setSubmitting(true);
    setAPIKeyLoginError('');
    try {
      await loginWithCPAAPIKey(apiKey);
      const session = await loadSession();
      if (!session.authenticated || session.role !== 'api_key_viewer') {
        setAPIKeyLoginError(t('auth.api_key_login_failed'));
        clearSession();
        return;
      }
      window.history.replaceState(null, '', appPath('/key-overview') + cpamcEmbedSearch());
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        setAPIKeyLoginError(t('auth.invalid_api_key'));
      } else if (error instanceof ApiError && error.status === 429) {
        setAPIKeyLoginError(t('auth.login_rate_limited'));
      } else {
        setAPIKeyLoginError(t('auth.api_key_login_failed'));
      }
      clearSession();
    } finally {
      setSubmitting(false);
    }
  }, [clearSession, loadSession, t]);

  let page: ReactNode;
  if (isShareMode && shareTab) {
    page = <UsagePage shareMode initialShareTab={shareTab} />;
  } else if (authState === 'checking') {
    page = <div className="app-checking" aria-busy="true" />;
  } else if (authState === 'unauthenticated') {
    page = <LoginPage loading={submitting} adminError={adminLoginError} apiKeyError={apiKeyLoginError} onPasswordSubmit={handlePasswordLogin} onAPIKeySubmit={handleAPIKeyLogin} />;
  } else if (authRole === 'api_key_viewer') {
    page = <KeyOverviewPage apiKey={sessionAPIKey} onAuthRequired={clearSession} />;
  } else {
    page = <UsagePage onAuthRequired={clearSession} />;
  }

  return (
    <div className="app-frame" data-embed={isEmbeddedInCPAMC ? 'cpamc' : undefined}>
      <main className="app-main">{page}</main>
      <AppFooter loadVersion={authState === 'authenticated'} />
    </div>
  );
}

export default App;
