type Entry = { value: string | null; expires: number };
export function remember(cache: Map<string, Entry>, key: string, value: string | null, now: number): void {
  cache.set(key, { value, expires: now + (value === null ? 5000 : 60000) });
}
