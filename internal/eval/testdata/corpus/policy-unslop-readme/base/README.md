# ratelimit

ratelimit is a token bucket rate limiter for Go services.

## Usage

Create a limiter with `ratelimit.New(rate, burst)` and call `Allow()` before each request.
