export async function readReplica(read: (host: string) => Promise<string>, primary: string, fallback: string): Promise<string> {
  try {
    return await read(primary);
  } catch {
    return read(fallback);
  }
}
