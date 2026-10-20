VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build test vet fmt-check check install clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/keepawake .

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

check: build vet fmt-check test

install:
	go install -ldflags "$(LDFLAGS)" .

clean:
	rm -rf bin
