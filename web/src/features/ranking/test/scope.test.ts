import { describe, expect, it, vi } from 'vitest';
import {
  DEFAULT_RANKING_SCOPE,
  loadRankingScope,
  normalizeRankingScope,
  persistRankingScope,
  RANKING_SCOPE_STORAGE_KEY,
} from '../scope';

const createStorage = (value: string | null = null) => ({
  getItem: vi.fn(() => value),
  setItem: vi.fn(),
});

describe('ranking scope persistence', () => {
  it('keeps only local and community values', () => {
    expect(normalizeRankingScope('local')).toBe('local');
    expect(normalizeRankingScope('community')).toBe('community');
    expect(normalizeRankingScope('unexpected')).toBeNull();
  });

  it('always loads local ranking while community scope is hidden', () => {
    expect(DEFAULT_RANKING_SCOPE).toBe('local');
    expect(loadRankingScope(undefined)).toBe('local');
    expect(loadRankingScope(createStorage('community'))).toBe('local');
    expect(loadRankingScope(createStorage('broken'))).toBe('local');
    expect(loadRankingScope({
      getItem: () => { throw new Error('blocked'); },
      setItem: vi.fn(),
    })).toBe('local');
  });

  it('stores the last explicit selection without failing the page', () => {
    const storage = createStorage();
    expect(persistRankingScope('local', storage)).toBe(true);
    expect(storage.setItem).toHaveBeenCalledWith(RANKING_SCOPE_STORAGE_KEY, 'local');
    expect(persistRankingScope('community', {
      getItem: vi.fn(),
      setItem: () => { throw new Error('blocked'); },
    })).toBe(false);
  });
});
