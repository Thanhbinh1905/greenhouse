export function delay(attempt: number, random: () => number): number {
  const cap = Math.min(30000, 100 * 2 ** attempt);
  return cap;
}
