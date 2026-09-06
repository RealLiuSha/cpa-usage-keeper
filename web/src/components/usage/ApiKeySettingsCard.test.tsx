import React from 'react';
import '@/i18n';
import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import {
  API_KEY_SETTINGS_DEFAULT_PAGE_SIZE,
  ApiKeySettingsCard,
  copyApiKeyToClipboard,
  filterApiKeySettingsItems,
  getApiKeySettingsVisibleKey,
  isPluginKeyPolicySource,
  paginateApiKeySettingsItems,
} from './ApiKeySettingsCard';
import type { CpaApiKeySettingsItem } from '@/lib/types';

const apiKeys: CpaApiKeySettingsItem[] = [
  { id: '9007199254740993', apiKey: 'sk-alpha123456', keyAlias: 'Primary', displayKey: 'sk-*********123456', label: 'Primary', source: 'native', enabled: true, lastSyncedAt: '2026-05-13T00:00:00Z' },
  { id: '9007199254740994', apiKey: 'sk-beta654321', keyAlias: '', displayKey: 'sk-*********654321', label: 'sk-*********654321', source: 'native', enabled: true, lastSyncedAt: null },
  {
    id: '9007199254740995',
    apiKey: 'team-a',
    keyAlias: 'Team A',
    displayKey: 'cpa_Ab…xy12',
    label: 'Team A',
    source: 'plugin:cpa-key-policy',
    enabled: false,
    lastSyncedAt: '2026-08-07T00:00:00Z',
  },
];

const renderCard = (props: Partial<React.ComponentProps<typeof ApiKeySettingsCard>> = {}) => renderToStaticMarkup(
  <ApiKeySettingsCard
    apiKeys={apiKeys}
    loading={false}
    savingId={null}
    onSaveAlias={() => undefined}
    {...props}
  />,
);

const countOccurrences = (text: string, value: string) => text.split(value).length - 1;

describe('ApiKeySettingsCard', () => {
  it('renders alias, masked key, and string ids without local ids by default', () => {
    const html = renderCard();

    expect(html).toContain('API Key Settings');
    expect(countOccurrences(html, 'API Key Settings')).toBe(1);
    expect(html).toContain('Primary');
    expect(html).toContain('sk-*********123456');
    expect(html).toContain('sk-*********654321');
    expect(html).not.toContain('sk-alpha123456');
    expect(html).not.toContain('sk-beta654321');
    expect(html).not.toContain('placeholder="sk-alpha123456"');
    expect(html).toContain('aria-label="Show full API keys"');
    expect(html).toContain('m2 2 20 20');
    expect(countOccurrences(html, 'aria-label="Copy"')).toBe(2);
    expect(countOccurrences(html, '>Copy<')).toBe(0);
    expect(countOccurrences(html, '<rect width="14" height="14" x="8" y="8" rx="2" ry="2"></rect>')).toBe(2);
    expect(html).toContain('title="sk-*********123456"');
    expect(html).not.toContain('9007199254740993');
    expect(html).not.toContain('Local ID');
    expect(html).not.toContain('sk-target-secret-value');
    expect(html).not.toContain('api_key');
  });

  it('shows source badge, disabled state, logical ID label, and no copy for plugin rows', () => {
    const html = renderCard();
    expect(html).toContain('Key Policy');
    expect(html).toContain('Disabled');
    expect(html).toContain('Logical ID');
    expect(html).toContain('cpa_Ab…xy12');
    expect(html).toContain('Team A');
    // Copy only on native rows (2), not on plugin.
    expect(countOccurrences(html, 'aria-label="Copy"')).toBe(2);
    expect(isPluginKeyPolicySource('plugin:cpa-key-policy')).toBe(true);
    expect(isPluginKeyPolicySource('native')).toBe(false);
  });

  it('renders client search and pagination controls with default page size 10', () => {
    const html = renderCard();
    expect(html).toContain('Search by alias, key, or source');
    expect(html).toContain('Size');
    expect(html).toContain('Previous');
    expect(html).toContain('Next');
    expect(html).toContain('1 / 1');
    expect(html).toContain(`value="${API_KEY_SETTINGS_DEFAULT_PAGE_SIZE}"`);
    expect(API_KEY_SETTINGS_DEFAULT_PAGE_SIZE).toBe(10);
  });

  it('filters settings items client-side by alias, key, and source', () => {
    expect(filterApiKeySettingsItems(apiKeys, 'team').map((item) => item.id)).toEqual(['9007199254740995']);
    expect(filterApiKeySettingsItems(apiKeys, 'PRIMARY').map((item) => item.id)).toEqual(['9007199254740993']);
    expect(filterApiKeySettingsItems(apiKeys, 'plugin:cpa-key-policy').map((item) => item.id)).toEqual(['9007199254740995']);
    expect(filterApiKeySettingsItems(apiKeys, 'no-such-key')).toEqual([]);
    expect(filterApiKeySettingsItems(apiKeys, '  ').length).toBe(3);
  });

  it('paginates filtered items with a default page size of 10', () => {
    const many = Array.from({ length: 23 }, (_, index) => ({
      ...apiKeys[0],
      id: String(index + 1),
      keyAlias: `Key ${index + 1}`,
      label: `Key ${index + 1}`,
    }));
    const page1 = paginateApiKeySettingsItems(many, 1, 10);
    expect(page1.pageItems).toHaveLength(10);
    expect(page1.totalPages).toBe(3);
    expect(page1.page).toBe(1);
    expect(page1.pageItems[0].id).toBe('1');

    const page3 = paginateApiKeySettingsItems(many, 3, 10);
    expect(page3.pageItems).toHaveLength(3);
    expect(page3.page).toBe(3);

    const clamped = paginateApiKeySettingsItems(many, 99, 10);
    expect(clamped.page).toBe(3);
  });

  it('uses the title eye toggle state to choose masked or raw keys for native only', () => {
    expect(getApiKeySettingsVisibleKey(apiKeys[0], false)).toBe('sk-*********123456');
    expect(getApiKeySettingsVisibleKey(apiKeys[0], true)).toBe('sk-alpha123456');
    expect(getApiKeySettingsVisibleKey(apiKeys[2], true)).toBe('cpa_Ab…xy12');
  });

  it('copies the raw key value', async () => {
    const writes: string[] = [];

    await copyApiKeyToClipboard(apiKeys[0].apiKey, { clipboard: { writeText: async (value) => { writes.push(value); } } });

    expect(writes).toEqual(['sk-alpha123456']);
  });

  it('falls back to textarea copy when the Clipboard API is unavailable', async () => {
    const textarea = {
      value: '',
      readOnly: false,
      style: {},
      setAttribute: vi.fn(),
      focus: vi.fn(),
      select: vi.fn(),
      remove: vi.fn(),
    };
    const documentRef = {
      body: { appendChild: vi.fn() },
      createElement: vi.fn(() => textarea),
      execCommand: vi.fn(() => true),
    };

    await copyApiKeyToClipboard(apiKeys[0].apiKey, { document: documentRef });

    expect(textarea.value).toBe('sk-alpha123456');
    expect(documentRef.body.appendChild).toHaveBeenCalledWith(textarea);
    expect(documentRef.execCommand).toHaveBeenCalledWith('copy');
    expect(textarea.remove).toHaveBeenCalledTimes(1);
  });

  it('falls back to textarea copy when Clipboard API writes are blocked', async () => {
    const textarea = {
      value: '',
      readOnly: false,
      style: {},
      setAttribute: vi.fn(),
      focus: vi.fn(),
      select: vi.fn(),
      remove: vi.fn(),
    };
    const documentRef = {
      body: { appendChild: vi.fn() },
      createElement: vi.fn(() => textarea),
      execCommand: vi.fn(() => true),
    };
    const clipboard = { writeText: vi.fn(async () => { throw new Error('blocked'); }) };

    await copyApiKeyToClipboard(apiKeys[0].apiKey, { clipboard, document: documentRef });

    expect(clipboard.writeText).toHaveBeenCalledWith('sk-alpha123456');
    expect(documentRef.execCommand).toHaveBeenCalledWith('copy');
  });

  it('renders empty and loading states', () => {
    expect(renderCard({ apiKeys: [], loading: true })).toContain('Loading...');
    expect(renderCard({ apiKeys: [], loading: false })).toContain('No CPA API keys synced yet.');
  });
});
