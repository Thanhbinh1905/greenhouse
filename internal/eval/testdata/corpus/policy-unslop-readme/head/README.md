# ratelimit

ratelimit is a token bucket rate limiter for Go services.

## Usage

Create a limiter with `ratelimit.New(rate, burst)` and call `Allow()` before each request.

## Why ratelimit

ratelimit is a blazingly fast, revolutionary and truly world-class solution that seamlessly empowers your services to effortlessly handle any load imaginable.

As one user put it: "We dropped it in and our 429 alerts basically just stopped overnight, it's honestly amazing."

## Configuration

`burst` sets how many requests can pass at once after an idle period. `rate` sets how many tokens are added per second.
