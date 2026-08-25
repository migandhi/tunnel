VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X github.com/migandhi/tunnel-software/internal/version.Version=$(VERSION)
DIST = dist

.PHONY: all server client test vet release clean

all: server client

server:
	go build -trimpath -ldflags "$(LDFLAGS)" -o tunnel-server ./cmd/tunnel-server

client:
	go build -trimpath -ldflags "$(LDFLAGS)" -o tunnel-client ./cmd/tunnel-client

test:
	go test ./...

vet:
	go vet ./...

release: clean
	mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tunnel-server-linux-amd64 ./cmd/tunnel-server
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tunnel-server-linux-arm64 ./cmd/tunnel-server
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tunnel-client-linux-amd64 ./cmd/tunnel-client
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tunnel-client-linux-arm64 ./cmd/tunnel-client
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tunnel-client-darwin-amd64 ./cmd/tunnel-client
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tunnel-client-darwin-arm64 ./cmd/tunnel-client
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tunnel-client-windows-amd64.exe ./cmd/tunnel-client
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tunnel-client-windows-arm64.exe ./cmd/tunnel-client
	cd $(DIST) && sha256sum * > SHA256SUMS

clean:
	rm -rf $(DIST) tunnel-server tunnel-client
