export function touch(cache: Map<string, string>, key: string): string | undefined {
  return cache.get(key);
}
