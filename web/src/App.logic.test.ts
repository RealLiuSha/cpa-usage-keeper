import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { getRoleHomePath, shouldNormalizeRolePath } from './App';

const appSource = readFileSync(new URL('./App.tsx', import.meta.url), 'utf8');
const appStylesSource = readFileSync(new URL('./App.css', import.meta.url), 'utf8');

describe('App role route normalization', () => {
  it('normalizes restored admin sessions away from the API Key viewer route', () => {
    expect(getRoleHomePath('admin')).toBe('/');
    expect(shouldNormalizeRolePath('admin', '/key-overview')).toBe(true);
    expect(shouldNormalizeRolePath('admin', '/')).toBe(false);
  });

  it('normalizes restored API Key viewer sessions to the key overview route', () => {
    expect(getRoleHomePath('api_key_viewer')).toBe('/key-overview');
    expect(shouldNormalizeRolePath('api_key_viewer', '/')).toBe(true);
    expect(shouldNormalizeRolePath('api_key_viewer', '/key-overview')).toBe(false);
  });

  it('does not bounce share dashboard paths to the role home when share public is enabled', () => {
    expect(shouldNormalizeRolePath('admin', '/share/overview', { sharePublicEnabled: true })).toBe(false);
    expect(shouldNormalizeRolePath('admin', '/share/analysis', { sharePublicEnabled: true })).toBe(false);
    expect(shouldNormalizeRolePath('admin', '/share/ranking', { sharePublicEnabled: true })).toBe(false);
  });

  it('normalizes share paths to role home when share public is disabled', () => {
    expect(shouldNormalizeRolePath('admin', '/share/overview', { sharePublicEnabled: false })).toBe(true);
    expect(shouldNormalizeRolePath('admin', '/', { sharePublicEnabled: false })).toBe(false);
  });

  it('renders UsagePage in share mode without waiting for a session', () => {
    expect(appSource).toContain('const isShareMode = Boolean(shareTab)');
    expect(appSource).toContain('<UsagePage shareMode initialShareTab={shareTab} />');
    expect(appSource).toMatch(/if \(isShareMode\) return;/);
  });

  it('does not use mutable share API flags or StrictMode cleanup flips', () => {
    expect(appSource).toContain('prepareShareModeFromLocation(window.location.pathname, window.__APP_BASE_PATH__)');
    expect(appSource).not.toContain('setSharePublicAPIEnabled');
    expect(appSource).toContain('Pure path + server feature flag');
  });

  it('clears stale overview auth errors when the session is cleared', () => {
    expect(appSource).toContain("import { useUsageStatsStore } from './stores/useUsageStatsStore';");
    expect(appSource).toMatch(/const clearUsageStats = useUsageStatsStore\(\(state\) => state\.clearUsageStats\);/);
    expect(appSource).toMatch(/const clearSession = useCallback\(\(\) => \{[\s\S]*?clearUsageStats\(\);[\s\S]*?setAuthState\('unauthenticated'\);/);
  });

  it('does not mount a global app footer in the shell', () => {
    expect(appSource).toContain("import './App.css';");
    expect(appSource).not.toContain("import { AppFooter } from './components/AppFooter';");
    expect(appSource).not.toContain('<AppFooter');
    expect(appSource).toMatch(/<div className="app-frame"[^>]*>[\s\S]*<main className="app-main">\{page\}<\/main>[\s\S]*<\/div>/);
  });

  it('lets app pages fill the viewport shell without a shared footer', () => {
    expect(appStylesSource).toMatch(/\.app-main\s*\{[\s\S]*?display:\s*flex;/);
    expect(appStylesSource).toMatch(/\.app-main\s*\{[\s\S]*?flex-direction:\s*column;/);
  });
});
