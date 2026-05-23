# Requires Go 1.23+ (see go.mod). With Go 1.22, set GOTOOLCHAIN=auto to fetch the toolchain.
export GOTOOLCHAIN ?= auto

.PHONY: build test race vet lint e2e clean

build: vet
	go build -o taskmasterd ./cmd/daemon
	go build -o taskmasterctl ./cmd/ctl

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

e2e:
	go test -v ./e2e -count=1 -timeout 120s

clean:
	rm -f taskmasterd taskmasterctl


