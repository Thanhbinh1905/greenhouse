export function nextPoll(hidden: boolean, errorCount: number): number {
  if (hidden) return 10000;
  return Math.min(30000, 1000 * 2 ** errorCount);
}
