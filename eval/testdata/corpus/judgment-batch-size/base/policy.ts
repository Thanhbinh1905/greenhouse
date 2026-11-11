export async function sendAll(values: string[], send: (value: string) => Promise<void>): Promise<void> {
  for (const value of values) await send(value);
}
