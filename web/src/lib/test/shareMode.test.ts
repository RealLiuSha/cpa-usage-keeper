import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  isActiveShareLocation,
  isSharePublicFeatureEnabled,
  isShareTab,
  prepareShareModeFromLocation,
  resolveShareTab,
  sharePathForTab,
  SHARE_TABS,
  stripAppBasePath,
} from '../shareMode';
import { apiPath, isSharePublicAPIEnabled } from '../api';
import { getUsageTabOptions } from '@/pages/UsagePage';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('share mode path detection', () => {
  it('resolves the three share dashboard tabs only', () => {
    expect(SHARE_TABS).toEqual(['overview', 'analysis', 'ranking']);
    expect(resolveShareTab('/share/overview')).toBe('overview');
    expect(resolveShareTab('/share/analysis/')).toBe('analysis');
    expect(resolveShareTab('share/ranking')).toBe('ranking');
    expect(resolveShareTab('/share/events')).toBeNull();
    expect(resolveShareTab('/share/settings')).toBeNull();
    expect(isShareTab('overview')).toBe(true);
    expect(sharePathForTab('analysis')).toBe('/share/analysis');
    expect(stripAppBasePath('/keeper/share/overview', '/keeper')).toBe('/share/overview');
  });

  it('requires the server feature flag before entering share mode', () => {
    expect(prepareShareModeFromLocation('/keeper/share/overview', '/keeper', false)).toBeNull();
    expect(prepareShareModeFromLocation('/keeper/share/overview', '/keeper', true)).toBe('overview');
    expect(isSharePublicFeatureEnabled(true)).toBe(true);
    expect(isSharePublicFeatureEnabled(false)).toBe(false);
    expect(isSharePublicFeatureEnabled(undefined)).toBe(false);
  });
});

describe('share public API routing', () => {
  it('prefixes share-safe reads with /public only when feature+path are active', () => {
    vi.stubGlobal('window', {
      __APP_BASE_PATH__: '/keeper',
      __SHARE_PUBLIC_ENABLED__: true,
      location: { pathname: '/keeper/share/overview' },
    });
    expect(isSharePublicAPIEnabled()).toBe(true);
    expect(apiPath('/usage/overview')).toBe('/keeper/api/v1/public/usage/overview');
    expect(apiPath('/ranking/local/leaderboards?period=today&metric=overall')).toBe(
      '/keeper/api/v1/public/ranking/local/leaderboards?period=today&metric=overall',
    );
    expect(apiPath('/usage/api-keys/settings')).toBe('/keeper/api/v1/usage/api-keys/settings');
    expect(apiPath('/usage/events')).toBe('/keeper/api/v1/usage/events');
  });

  it('does not use public prefix when feature flag is off even on share path', () => {
    vi.stubGlobal('window', {
      __APP_BASE_PATH__: '/keeper',
      __SHARE_PUBLIC_ENABLED__: false,
      location: { pathname: '/keeper/share/overview' },
    });
    expect(isActiveShareLocation()).toBe(false);
    expect(apiPath('/usage/overview')).toBe('/keeper/api/v1/usage/overview');
  });

  it('enables public prefix from location without mutable setters (M1)', () => {
    vi.stubGlobal('window', {
      __APP_BASE_PATH__: '/keeper',
      __SHARE_PUBLIC_ENABLED__: true,
      location: { pathname: '/keeper/share/analysis' },
    });
    // No setSharePublicAPIEnabled — pure location derivation.
    expect(prepareShareModeFromLocation(window.location.pathname, window.__APP_BASE_PATH__)).toBe('analysis');
    expect(apiPath('/usage/analysis')).toBe('/keeper/api/v1/public/usage/analysis');
    expect(apiPath('/ranking/local/leaderboards?period=today&metric=overall')).toBe(
      '/keeper/api/v1/public/ranking/local/leaderboards?period=today&metric=overall',
    );
  });

  it('routes fetchLocalRankingLeaderboard through public prefix when share mode is on', async () => {
    vi.stubGlobal('window', {
      __APP_BASE_PATH__: '/keeper',
      __SHARE_PUBLIC_ENABLED__: true,
      location: { pathname: '/keeper/share/ranking' },
    });
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      period: 'today',
      metric: 'overall',
      entries: [],
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);

    const { fetchLocalRankingLeaderboard } = await import('@/features/ranking/api');
    await fetchLocalRankingLeaderboard('today', 'overall');

    const requested = String(fetchMock.mock.calls[0]?.[0] ?? '');
    expect(requested).toContain('/keeper/api/v1/public/ranking/local/leaderboards?period=today&metric=overall');
  });

  it('keeps public API routing while tab URL stays under /share/*', () => {
    const location = { pathname: '/keeper/share/overview' };
    vi.stubGlobal('window', {
      __APP_BASE_PATH__: '/keeper',
      __SHARE_PUBLIC_ENABLED__: true,
      location,
    });
    expect(apiPath('/usage/overview')).toContain('/public/');
    // Simulate handleTabChange writing another share tab path.
    location.pathname = '/keeper/share/ranking';
    expect(apiPath('/ranking/local/leaderboards?period=today&metric=overall')).toContain('/public/');
    // Leaving /share/* silently returns to admin API surface — invariant for future routing.
    location.pathname = '/keeper/';
    expect(apiPath('/usage/overview')).toBe('/keeper/api/v1/usage/overview');
  });
});

describe('share mode tab options', () => {
  it('only exposes overview, analysis, and ranking', () => {
    const values = getUsageTabOptions((key) => key, { shareMode: true }).map((option) => option.value);
    expect(values).toEqual(['overview', 'analysis', 'ranking']);
    expect(values).not.toContain('events');
    expect(values).not.toContain('auth-files');
    expect(values).not.toContain('ai-provider');
    expect(values).not.toContain('settings');
  });
});
