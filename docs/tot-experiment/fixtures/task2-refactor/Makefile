.PHONY: test vet metrics

test:
	go test ./... -v -count=1

vet:
	go vet ./...

metrics:
	@echo "=== File line counts ==="
	@find . -name '*.go' -not -path './vendor/*' | xargs wc -l | sort -n
	@echo "=== Package count ==="
	@find . -name '*.go' -not -path './vendor/*' -exec dirname {} \; | sort -u | wc -l
