import { RANKING_SCOPES, type RankingScope } from './types';

export const RANKING_SCOPE_STORAGE_KEY = 'cli-proxy-usage-ranking-scope-v1';
// 产品面仅展示本地榜；社区榜入口关闭后默认与强制加载均固定为 local。
export const DEFAULT_RANKING_SCOPE: RankingScope = 'local';

interface RankingScopeStorage {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
}

export const normalizeRankingScope = (value: unknown): RankingScope | null => (
  typeof value === 'string' && RANKING_SCOPES.includes(value as RankingScope)
    ? value as RankingScope
    : null
);

export const loadRankingScope = (
  _storage: RankingScopeStorage | undefined = typeof localStorage === 'undefined' ? undefined : localStorage,
): RankingScope => {
  // 忽略历史 localStorage（含 community），避免用户仍落到已隐藏的社区榜。
  return DEFAULT_RANKING_SCOPE;
};

export const persistRankingScope = (
  scope: RankingScope,
  storage: RankingScopeStorage | undefined = typeof localStorage === 'undefined' ? undefined : localStorage,
): boolean => {
  try {
    storage?.setItem(RANKING_SCOPE_STORAGE_KEY, scope);
    return Boolean(storage);
  } catch {
    return false;
  }
};
