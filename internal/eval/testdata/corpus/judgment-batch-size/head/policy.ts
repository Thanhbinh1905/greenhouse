export async function sendAll(values: string[], send: (value: string) => Promise<void>): Promise<void> {
  await Promise.all(values.map(value => send(value)));
}
