.PHONY: build run sidecar sidecar-venv deps clean

BIN := bin/voxrobota

build: ## compile the Go binary
	go build -o $(BIN) .

run: build ## build then run the TUI (sidecar must be running)
	./$(BIN)

deps: ## download Go module dependencies
	go mod tidy

sidecar-venv: ## create the python venv and install STT/TTS deps + models
	./setup.sh sidecar

sidecar: ## run the STT/TTS sidecar (blocks; run in its own terminal)
	.venv/bin/python sidecar/server.py

clean:
	rm -rf bin

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	 awk 'BEGIN{FS=":.*?## "}{printf "  %-14s %s\n", $$1, $$2}'
