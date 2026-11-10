export function encoding(body: Uint8Array, acceptsGzip: boolean): "gzip" | "identity" {
  return acceptsGzip && body.byteLength >= 1024 ? "gzip" : "identity";
}
