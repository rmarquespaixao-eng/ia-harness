APP_NAME := ia-harness

.PHONY: build test vet fmt-check staticcheck vuln generate verify clean

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l .)"

staticcheck:
	go tool staticcheck ./...

vuln:
	go tool govulncheck ./...

generate:
	go generate ./...

# Gate local pré-merge (constitution §7): fmt + vet + staticcheck + generate
# (idempotente) + testes (inclui a conformidade do gerado) + build + vuln.
verify: fmt-check vet staticcheck generate test build vuln

clean:
	rm -rf bin
