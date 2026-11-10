export async function readReplica(read: (host: string) => Promise<string>, primary: string, fallback: string): Promise<string> {
  return read(primary);
}
