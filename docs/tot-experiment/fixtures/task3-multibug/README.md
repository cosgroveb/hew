# Bug Fix: Concurrent TTL Cache

The `cache.go` file implements a concurrent cache with TTL expiration and LRU eviction.
It has 3 bugs that cause test failures.

## Task

Fix all bugs in `cache.go` so that ALL tests pass, including with `-race`.

## Constraints

- Do NOT modify `cache_test.go` or `working_test.go`
- Only modify `cache.go`
- All tests must pass: `make test`
- Tests in `working_test.go` must continue to pass (no regressions)

## Hints

- The 3 bugs are interrelated. Fixing one may affect another.
- Run `make test` to see which tests fail and why.
- Run `go test -race ./...` to detect race conditions.
