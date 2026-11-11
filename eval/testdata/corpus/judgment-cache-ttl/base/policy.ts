type Entry = { value: string | null; expires: number };
export function remember(cache: Map<string, Entry>, key: string, value: string | null, now: number): void {
  if (value === null) return;
  cache.set(key, { value, expires: now + 60000 });
}
