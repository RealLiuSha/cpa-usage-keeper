// @vitest-environment happy-dom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { CpaApiKeySettingsItem } from '@/lib/types'
import { ApiKeySettingsCard } from '../ApiKeySettingsCard'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

const translations: Record<string, string> = {
  'usage_stats.api_key_settings_plugin_identity': 'Third-party Identity #{{id}}',
  'usage_stats.api_key_settings_plugin': 'Third-party',
  'usage_stats.api_key_settings_native': 'Native',
  'usage_stats.api_key_enabled_false': 'Disabled',
  'usage_stats.api_key_settings_search_placeholder': 'Search',
  'usage_stats.next_page': 'Next',
}

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => undefined },
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string>) => (translations[key] ?? key).replace('{{id}}', values?.id ?? ''),
  }),
}))

const keyPolicyKey = (id: number, overrides: Partial<CpaApiKeySettingsItem> = {}): CpaApiKeySettingsItem => ({
  id: String(id),
  sourceType: 'plugin',
  accessProvider: 'plugin:cpa-key-policy',
  keyAlias: `Team ${id}`,
  displayKey: `cpa_${id}…xy12`,
  label: `Team ${id}`,
  enabled: true,
  lastSyncedAt: null,
  ...overrides,
})

describe('ApiKeySettingsCard key-policy catalog rows', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  const render = async (apiKeys: CpaApiKeySettingsItem[]) => {
    await act(async () => root.render(<ApiKeySettingsCard apiKeys={apiKeys} onSaveAlias={vi.fn()} />))
  }
  const cardNames = () => Array.from(container.querySelectorAll('[class*="apiKeySettingsName"]'), (node) => node.textContent)

  it('shows the catalog preview and disabled state without copy or reveal actions', async () => {
    await render([keyPolicyKey(1, { enabled: false })])

    expect(container.textContent).toContain('cpa_1…xy12')
    expect(container.textContent).not.toContain('Third-party Identity')
    expect(container.textContent).toContain('Disabled')
    expect(container.querySelector('[aria-label="Copy"], [aria-label="Show API key"]')).toBeNull()
    expect(container.querySelector('input')).toBeNull()
  })

  it('searches and pages once the list exceeds one page', async () => {
    await render(Array.from({ length: 12 }, (_, index) => keyPolicyKey(index + 1)))

    expect(cardNames()).toHaveLength(10)
    const next = Array.from(container.querySelectorAll('button')).find((node) => node.textContent === 'Next')!
    await act(async () => next.click())
    expect(cardNames()).toHaveLength(2)

    const search = container.querySelector<HTMLInputElement>('input[aria-label="Search"]')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(search, 'Team 3')
      search.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(cardNames()).toHaveLength(1)
    expect(cardNames()[0]).toContain('Team 3')
  })
})
