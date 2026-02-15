VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build build-reconcile test vet fmt lint cross-compile docker clean

build:
	go build $(LDFLAGS) -o bin/bor ./cmd/bor

build-reconcile:
	go build $(LDFLAGS) -o bin/reconcile ./arbiter/cmd/reconcile

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

lint: vet
	@echo "lint passed"

cross-compile:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/bor-linux-amd64 ./cmd/bor
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/bor-darwin-amd64 ./cmd/bor
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/bor-darwin-arm64 ./cmd/bor
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/reconcile-linux-amd64 ./arbiter/cmd/reconcile

docker:
	docker build -t boxofrocks:$(VERSION) .

clean:
	rm -rf bin/
