# Rate Limiter

Implement a rate limiter that satisfies the `Limiter` interface in `limiter.go`.

## Requirements

- Respect `Config.Rate`: maximum requests per `Config.Window`
- Respect `Config.Burst`: allow burst up to this value (if 0, use Rate)
- Support per-key isolation
- Be safe for concurrent use
- Use the provided `Clock` interface (do not use `time.Now()` directly)

## Constraints

- Do NOT modify `limiter.go`, `testclock.go`, or `limiter_test.go`
- Only modify `impl.go`
- Run `make test` to verify

## Valid approaches

Any correct algorithm: token bucket, sliding window, fixed window, leaky bucket.
