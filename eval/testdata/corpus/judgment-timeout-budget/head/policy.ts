export async function fetchProfile(url: string): Promise<Response> {
  return fetch(url, { signal: AbortSignal.timeout(1500) });
}
