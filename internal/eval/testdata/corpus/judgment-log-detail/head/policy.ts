export function diagnostic(error: Error): string {
  return error.stack ?? error.message;
}
