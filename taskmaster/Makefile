# Requires Go 1.23+ (see go.mod). With Go 1.22, set GOTOOLCHAIN=auto to fetch the toolchain.
export GOTOOLCHAIN ?= auto

.PHONY: build test race vet lint e2e clean

BIN_DIR := bin

build: vet
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/taskmasterd ./cmd/daemon
	go build -o $(BIN_DIR)/taskmasterctl ./cmd/ctl

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
	rm -f $(BIN_DIR)/taskmasterd $(BIN_DIR)/taskmasterctl


