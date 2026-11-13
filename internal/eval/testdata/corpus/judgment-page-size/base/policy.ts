export function page<T>(values: readonly T[], limit: number): readonly T[] {
  return values.slice(0, Math.max(0, limit));
}
