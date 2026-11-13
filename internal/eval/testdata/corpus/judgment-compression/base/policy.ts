export function encoding(body: Uint8Array, acceptsGzip: boolean): "gzip" | "identity" {
  return acceptsGzip ? "gzip" : "identity";
}
