// @vitest-environment happy-dom

import { afterEach, describe, expect, it } from 'vitest';
import { getInitialLanguage } from '../index';

const LANGUAGE_STORAGE_KEY = 'cpa-usage-keeper-language';

afterEach(() => {
  window.localStorage.clear();
});

describe('getInitialLanguage', () => {
  it('defaults to Simplified Chinese when nothing is saved', () => {
    expect(getInitialLanguage()).toBe('zh');
  });

  it('keeps a saved preference, so changing the default cannot override returning users', () => {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, 'en');
    expect(getInitialLanguage()).toBe('en');

    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, 'zh-TW');
    expect(getInitialLanguage()).toBe('zh-TW');
  });

  it('falls back to the default when the saved value is not a supported language', () => {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, 'de');
    expect(getInitialLanguage()).toBe('zh');
  });
});
