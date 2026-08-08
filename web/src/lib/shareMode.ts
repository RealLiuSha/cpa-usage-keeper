export const SHARE_TABS = ['overview', 'analysis', 'ranking'] as const;

export type ShareTab = (typeof SHARE_TABS)[number];

const SHARE_PATH_PREFIX = '/share/';
const APP_BASE_PATH_PLACEHOLDER = '__APP_BASE_PATH__';

export const isShareTab = (value: unknown): value is ShareTab => (
  typeof value === 'string' && (SHARE_TABS as readonly string[]).includes(value)
);

/**
 * Resolve a share dashboard tab from a path already stripped of APP_BASE_PATH.
 * Accepts `/share/overview`, `/share/analysis`, `/share/ranking` (trailing slash ok).
 */
export const resolveShareTab = (pathname: string): ShareTab | null => {
  const normalized = pathname.startsWith('/') ? pathname : `/${pathname}`;
  if (!normalized.startsWith(SHARE_PATH_PREFIX)) return null;
  const rest = normalized.slice(SHARE_PATH_PREFIX.length).replace(/\/+$/, '');
  if (!rest || rest.includes('/')) return null;
  return isShareTab(rest) ? rest : null;
};

export const sharePathForTab = (tab: ShareTab): string => `${SHARE_PATH_PREFIX}${tab}`;

/** Strip APP_BASE_PATH from a browser pathname (same rules as the app shell). */
export const stripAppBasePath = (pathname: string, basePath: string | undefined): string => {
  if (!basePath || basePath === '/' || basePath === APP_BASE_PATH_PLACEHOLDER) return pathname || '/';
  const normalizedBase = basePath.endsWith('/') ? basePath.slice(0, -1) : basePath;
  if (!pathname.startsWith(normalizedBase)) return pathname || '/';
  const stripped = pathname.slice(normalizedBase.length);
  return stripped || '/';
};

/** Server injects a JS boolean into index.html; missing/placeholder means disabled. */
export const isSharePublicFeatureEnabled = (
  value: unknown = typeof window === 'undefined' ? undefined : window.__SHARE_PUBLIC_ENABLED__,
): boolean => value === true;

/**
 * Resolve share tab only when both path matches and the deployment enabled share public.
 * Pure: no mutable globals — apiPath re-reads location + feature flag each call (M1).
 */
export const prepareShareModeFromLocation = (
  pathname: string,
  basePath: string | undefined,
  featureEnabled: boolean = isSharePublicFeatureEnabled(),
): ShareTab | null => {
  if (!featureEnabled) return null;
  return resolveShareTab(stripAppBasePath(pathname, basePath));
};

/** True when the current browser location is an active share dashboard entry. */
export const isActiveShareLocation = (
  pathname: string | undefined = typeof window === 'undefined' ? undefined : window.location?.pathname,
  basePath: string | undefined = typeof window === 'undefined' ? undefined : window.__APP_BASE_PATH__,
  featureEnabled: boolean = isSharePublicFeatureEnabled(),
): boolean => {
  if (!pathname) return false;
  return prepareShareModeFromLocation(pathname, basePath, featureEnabled) !== null;
};
