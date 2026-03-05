BIN     := cu-watcher
MODULE  := cu-watcher
CMD     := ./cmd
DIST    := dist

VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: all build darwin linux windows clean test

all: darwin linux windows

darwin:
	@mkdir -p $(DIST)
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-darwin-amd64   $(CMD)
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-darwin-arm64   $(CMD)

linux:
	@mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-linux-amd64    $(CMD)
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-linux-arm64    $(CMD)

windows:
	@mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-windows-amd64.exe $(CMD)

build:
	go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN) $(CMD)

test:
	go test ./internal/parse/ -v

clean:
	rm -rf $(DIST)
