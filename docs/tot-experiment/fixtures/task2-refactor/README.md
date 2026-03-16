# Refactor: UserOrderService

The `service/service.go` file is a 300-line god object that handles user management,
order processing, notifications, and validation all in one struct.

## Task

Refactor `service/service.go` to reduce coupling. You may:
- Split into multiple files within `service/` package
- Extract new packages (e.g., `notification/`, `validation/`)
- Introduce interfaces for dependency injection
- Move methods to separate types

## Constraints

- Do NOT modify `types/types.go` or `service_test.go`
- All tests must continue to pass: `make test`
- The public API (`NewUserOrderService()` and all exported methods) must remain unchanged
- No file should exceed 150 lines after refactoring

## Success criteria

- All tests pass
- No file > 150 lines
- At least 3 packages (currently 2: service, types)
- `go vet` clean
