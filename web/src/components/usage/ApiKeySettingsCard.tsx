import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { IconCheck, IconCopy, IconEye, IconEyeOff } from '@/components/ui/icons';
import { useScrollBoundaryContainment } from '@/hooks/useScrollBoundaryContainment';
import type { CpaApiKeySettingsItem, CpaApiKeySource } from '@/lib/types';
import styles from '@/pages/UsagePage.module.scss';

type ClipboardWriter = Pick<Clipboard, 'writeText'>;
type CopyTextArea = {
  value: string;
  readOnly: boolean;
  style: {
    position?: string;
    opacity?: string;
    pointerEvents?: string;
    top?: string;
    left?: string;
  };
  setAttribute: (name: string, value: string) => void;
  focus: () => void;
  select: () => void;
  remove?: () => void;
};
type CopyDocument = {
  activeElement?: { isConnected: boolean; focus: (options?: FocusOptions) => void } | null;
  body?: {
    appendChild: (node: CopyTextArea) => unknown;
    removeChild?: (node: CopyTextArea) => unknown;
  };
  createElement?: (tagName: 'textarea') => CopyTextArea;
  execCommand?: (command: string) => boolean;
};
type CopyContext = {
  clipboard?: ClipboardWriter;
  document?: CopyDocument;
};

export const CPA_API_KEY_SOURCE_PLUGIN_KEY_POLICY = 'plugin:cpa-key-policy';
export const API_KEY_SETTINGS_DEFAULT_PAGE_SIZE = 10;
export const API_KEY_SETTINGS_PAGE_SIZE_OPTIONS = [10, 20, 50] as const;

export function isPluginKeyPolicySource(source?: CpaApiKeySource | null): boolean {
  return source === CPA_API_KEY_SOURCE_PLUGIN_KEY_POLICY;
}

export function getApiKeySettingsVisibleKey(item: CpaApiKeySettingsItem, showFullApiKeys: boolean) {
  // Plugin rows store a logical id, not a secret — never treat show-full as secret reveal.
  if (isPluginKeyPolicySource(item.source)) {
    return item.displayKey || item.apiKey;
  }
  return showFullApiKeys && item.apiKey ? item.apiKey : item.displayKey;
}

/** Client-side search over alias, labels, display/raw key values (case-insensitive). */
export function filterApiKeySettingsItems(
  items: CpaApiKeySettingsItem[],
  query: string,
): CpaApiKeySettingsItem[] {
  const needle = query.trim().toLowerCase();
  if (!needle) {
    return items;
  }
  return items.filter((item) => {
    const haystack = [
      item.keyAlias,
      item.label,
      item.displayKey,
      item.apiKey,
      item.source ?? '',
    ]
      .join('\0')
      .toLowerCase();
    return haystack.includes(needle);
  });
}

export function paginateApiKeySettingsItems<T>(
  items: T[],
  page: number,
  pageSize: number,
): { pageItems: T[]; page: number; totalPages: number } {
  const size = pageSize > 0 ? pageSize : API_KEY_SETTINGS_DEFAULT_PAGE_SIZE;
  const totalPages = Math.max(1, Math.ceil(items.length / size) || 1);
  const safePage = Math.min(Math.max(page, 1), totalPages);
  const start = (safePage - 1) * size;
  return {
    pageItems: items.slice(start, start + size),
    page: safePage,
    totalPages,
  };
}

export async function copyApiKeyToClipboard(apiKey: string, context: CopyContext = {}) {
  if (!apiKey) {
    return;
  }
  const clipboard = context.clipboard ?? globalThis.navigator?.clipboard;
  if (clipboard) {
    try {
      await clipboard.writeText(apiKey);
      return;
    } catch {
      // HTTP LAN pages can block navigator.clipboard; fall back to a selected textarea copy.
    }
  }
  const documentRef = context.document ?? (typeof document !== 'undefined' ? document as unknown as CopyDocument : undefined);
  const textarea = documentRef?.createElement?.('textarea');
  if (!documentRef?.body || !documentRef.execCommand || !textarea) {
    throw new Error('clipboard is not available');
  }
  textarea.value = apiKey;
  textarea.readOnly = true;
  textarea.setAttribute('aria-hidden', 'true');
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  textarea.style.pointerEvents = 'none';
  textarea.style.top = '0';
  textarea.style.left = '0';
  const previouslyFocused = documentRef.activeElement;
  documentRef.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  try {
    if (!documentRef.execCommand('copy')) {
      throw new Error('copy command failed');
    }
  } finally {
    if (textarea.remove) {
      textarea.remove();
    } else {
      documentRef.body.removeChild?.(textarea);
    }
    // 兼容复制借用焦点后还给原控件，避免一次复制打断页面的键盘操作。
    if (previouslyFocused?.isConnected) previouslyFocused.focus({ preventScroll: true });
  }
}

export interface ApiKeySettingsCardProps {
  apiKeys: CpaApiKeySettingsItem[];
  loading?: boolean;
  savingId?: string | null;
  onSaveAlias: (id: string, keyAlias: string) => void | Promise<void>;
  onNotice?: (kind: 'success' | 'info' | 'error', message: string) => void;
}

export function ApiKeySettingsCard({ apiKeys, loading = false, savingId = null, onSaveAlias, onNotice }: ApiKeySettingsCardProps) {
  const { t } = useTranslation();
  const [showFullApiKeys, setShowFullApiKeys] = useState(false);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(API_KEY_SETTINGS_DEFAULT_PAGE_SIZE);
  const copyResetTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const apiKeySettingsBodyRef = useRef<HTMLDivElement | null>(null);
  useScrollBoundaryContainment(apiKeySettingsBodyRef);
  const initialAliases = useMemo(
    () => Object.fromEntries(apiKeys.map((item) => [item.id, item.keyAlias])),
    [apiKeys],
  );
  const [draftAliases, setDraftAliases] = useState<Record<string, string>>(initialAliases);

  useEffect(() => {
    setDraftAliases(initialAliases);
  }, [initialAliases]);

  useEffect(() => () => {
    if (copyResetTimerRef.current) {
      clearTimeout(copyResetTimerRef.current);
    }
  }, []);

  const filteredKeys = useMemo(
    () => filterApiKeySettingsItems(apiKeys, searchQuery),
    [apiKeys, searchQuery],
  );
  const { pageItems, page: safePage, totalPages } = useMemo(
    () => paginateApiKeySettingsItems(filteredKeys, page, pageSize),
    [filteredKeys, page, pageSize],
  );

  useEffect(() => {
    if (safePage !== page) {
      setPage(safePage);
    }
  }, [page, safePage]);

  const handleSearchChange = useCallback((value: string) => {
    setSearchQuery(value);
    setPage(1);
  }, []);

  const handlePageSizeChange = useCallback((size: number) => {
    setPageSize(size);
    setPage(1);
  }, []);

  const handleCopyApiKey = useCallback(async (item: CpaApiKeySettingsItem) => {
    if (isPluginKeyPolicySource(item.source)) {
      return;
    }
    try {
      await copyApiKeyToClipboard(item.apiKey);
      setCopiedId(item.id);
      onNotice?.('success', t('usage_stats.api_key_settings_copy_success'));
      if (copyResetTimerRef.current) {
        clearTimeout(copyResetTimerRef.current);
      }
      copyResetTimerRef.current = setTimeout(() => setCopiedId(null), 1600);
    } catch {
      setCopiedId(null);
      onNotice?.('error', t('usage_stats.api_key_settings_copy_failed'));
    }
  }, [onNotice, t]);
  const toggleLabel = showFullApiKeys
    ? t('usage_stats.api_key_settings_hide_full')
    : t('usage_stats.api_key_settings_show_full');

  return (
    <Card
      title={t('usage_stats.api_key_settings_title')}
      subtitle={t('usage_stats.api_key_settings_subtitle')}
      titleMeta={
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={`${styles.apiKeyVisibilityToggle} ${showFullApiKeys ? styles.apiKeyVisibilityToggleActive : ''}`.trim()}
          onClick={() => setShowFullApiKeys((current) => !current)}
          aria-label={toggleLabel}
          aria-pressed={showFullApiKeys}
          title={toggleLabel}
        >
          {showFullApiKeys ? <IconEye size={16} /> : <IconEyeOff size={16} />}
        </Button>
      }
      className={`${styles.detailsFixedCard} ${styles.apiKeySettingsCard}`}
    >
      <div ref={apiKeySettingsBodyRef} className={styles.apiKeySettingsBody}>
        {loading && apiKeys.length === 0 ? (
          <div className={styles.hint}>{t('common.loading')}</div>
        ) : apiKeys.length === 0 ? (
          <div className={styles.hint}>{t('usage_stats.api_key_settings_empty')}</div>
        ) : (
          <>
            <div className={styles.apiKeySettingsToolbar}>
              <Input
                value={searchQuery}
                onChange={(event) => handleSearchChange(event.target.value)}
                placeholder={t('usage_stats.api_key_settings_search_placeholder')}
                aria-label={t('usage_stats.api_key_settings_search_placeholder')}
                className={`${styles.usagePillControl} ${styles.apiKeySettingsSearchInput}`.trim()}
              />
            </div>
            {filteredKeys.length === 0 ? (
              <div className={styles.hint}>{t('usage_stats.api_key_settings_search_empty')}</div>
            ) : (
              <>
                <div className={styles.apiKeySettingsList}>
                  {pageItems.map((item) => {
                    const draftAlias = draftAliases[item.id] ?? '';
                    const disabled = savingId === item.id;
                    const plugin = isPluginKeyPolicySource(item.source);
                    const apiKey = getApiKeySettingsVisibleKey(item, showFullApiKeys);
                    const copyLabel = copiedId === item.id ? t('usage_stats.api_key_settings_copied') : t('usage_stats.api_key_settings_copy');
                    const fieldLabel = plugin
                      ? t('usage_stats.api_key_settings_logical_id')
                      : t('usage_stats.api_key_settings_display_key');
                    const sourceLabel = plugin
                      ? t('usage_stats.api_key_source_key_policy')
                      : t('usage_stats.api_key_source_native');
                    const isDisabledPlugin = plugin && item.enabled === false;
                    return (
                      <div
                        key={item.id}
                        className={`${styles.apiKeySettingsItem}${isDisabledPlugin ? ` ${styles.apiKeySettingsItemDisabled}` : ''}`.trim()}
                      >
                        <div className={styles.apiKeySettingsSummary}>
                          <span className={styles.apiKeyFieldLabel}>{fieldLabel}</span>
                          <div className={styles.apiKeySettingsNameRow}>
                            <span className={styles.apiKeySettingsName} title={apiKey}>{apiKey}</span>
                            {!plugin ? (
                              <button
                                type="button"
                                className={`${styles.apiKeySettingsCopyIconButton} ${copiedId === item.id ? styles.apiKeySettingsCopyIconButtonCopied : ''}`.trim()}
                                onClick={() => void handleCopyApiKey(item)}
                                disabled={!item.apiKey}
                                aria-label={copyLabel}
                                title={copyLabel}
                              >
                                {copiedId === item.id ? <IconCheck size={14} /> : <IconCopy size={14} />}
                              </button>
                            ) : null}
                          </div>
                          <span className={styles.apiKeySourceBadge} data-source={plugin ? 'plugin' : 'native'}>
                            {sourceLabel}
                          </span>
                          {isDisabledPlugin ? (
                            <span className={styles.apiKeyDisabledBadge}>{t('usage_stats.api_key_enabled_false')}</span>
                          ) : null}
                        </div>
                        <div className={styles.apiKeySettingsForm}>
                          <label className={styles.apiKeyAliasField}>
                            <span className={styles.apiKeyAliasLabel}>{t('usage_stats.api_key_settings_alias')}</span>
                            <Input
                              value={draftAlias}
                              onChange={(event) => setDraftAliases((current) => ({ ...current, [item.id]: event.target.value }))}
                              placeholder={apiKey}
                              aria-label={`${t('usage_stats.api_key_settings_alias')} ${apiKey}`}
                              className={`${styles.usagePillControl} ${styles.apiKeyAliasInput}`.trim()}
                              disabled={disabled}
                            />
                          </label>
                          <div className={styles.apiKeySettingsActions}>
                            <Button
                              variant="primary"
                              size="sm"
                              appearance="action"
                              className={styles.apiKeySettingsSaveButton}
                              onClick={() => onSaveAlias(item.id, draftAlias)}
                              disabled={disabled}
                            >
                              {disabled ? t('usage_stats.api_key_settings_saving') : t('common.save')}
                            </Button>
                          </div>
                        </div>
                      </div>
                    );
                  })}
                </div>
                {filteredKeys.length > 0 ? (
                  <div className={styles.apiKeySettingsPagination}>
                    <div className={styles.apiKeySettingsPaginationControls}>
                      <label className={styles.apiKeySettingsPageSizeControl}>
                        <span>{t('usage_stats.rows_per_page')}</span>
                        <select
                          value={pageSize}
                          onChange={(event) => handlePageSizeChange(Number(event.target.value))}
                          aria-label={t('usage_stats.rows_per_page')}
                        >
                          {API_KEY_SETTINGS_PAGE_SIZE_OPTIONS.map((option) => (
                            <option key={option} value={option}>{option}</option>
                          ))}
                        </select>
                      </label>
                      <button
                        type="button"
                        onClick={() => setPage((current) => Math.max(1, current - 1))}
                        disabled={safePage <= 1}
                      >
                        {t('usage_stats.previous_page')}
                      </button>
                      <span className={styles.apiKeySettingsPaginationPage}>
                        {safePage} / {totalPages}
                      </span>
                      <button
                        type="button"
                        onClick={() => setPage((current) => Math.min(totalPages, current + 1))}
                        disabled={safePage >= totalPages}
                      >
                        {t('usage_stats.next_page')}
                      </button>
                    </div>
                  </div>
                ) : null}
              </>
            )}
          </>
        )}
      </div>
    </Card>
  );
}
