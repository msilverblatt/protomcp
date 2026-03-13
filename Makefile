.PHONY: proto build test clean

PROTO_DIR := proto
GEN_DIR := gen/proto/protomcp
PYTHON_GEN_DIR := sdk/python/gen
TS_GEN_DIR := sdk/typescript/gen

proto:
	mkdir -p $(GEN_DIR) $(PYTHON_GEN_DIR) $(TS_GEN_DIR)
	protoc --go_out=$(GEN_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(GEN_DIR) --go-grpc_opt=paths=source_relative \
		-I$(PROTO_DIR) $(PROTO_DIR)/protomcp.proto
	protoc --python_out=$(PYTHON_GEN_DIR) \
		-I$(PROTO_DIR) $(PROTO_DIR)/protomcp.proto
	protoc --plugin=protoc-gen-ts=$$(which protoc-gen-ts) \
		--ts_out=$(TS_GEN_DIR) \
		-I$(PROTO_DIR) $(PROTO_DIR)/protomcp.proto

build:
	go build -o bin/pmcp ./cmd/protomcp

test:
	go test ./...

test-python:
	cd sdk/python && python -m pytest tests/ -v

test-ts:
	cd sdk/typescript && npx vitest run

test-all: test test-python test-ts

clean:
	rm -rf bin/ gen/
