BIN        ?= perforce-exporter
PKG        := ./cmd/perforce-exporter
LDFLAGS    := -s -w
BUILD_OPTS := -trimpath -ldflags="$(LDFLAGS)"

DIST := dist

.PHONY: all build test vet tidy run clean dist

all: vet test build

build:
	CGO_ENABLED=0 go build $(BUILD_OPTS) -o $(BIN) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

run: build
	PERFORCE_TARGETS="$${PERFORCE_TARGETS:-primary=127.0.0.1:1666,replica=127.0.0.1:1667}" \
		P4_BIN="$${P4_BIN:-p4}" \
		./$(BIN)

# Release builds for the platforms we care about. The Linux/amd64 binary is
# the one that ships next to p4d on the Perforce admin host.
dist:
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build $(BUILD_OPTS) -o $(DIST)/$(BIN)-linux-amd64        $(PKG)
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build $(BUILD_OPTS) -o $(DIST)/$(BIN)-linux-arm64        $(PKG)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build $(BUILD_OPTS) -o $(DIST)/$(BIN)-darwin-arm64       $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(BUILD_OPTS) -o $(DIST)/$(BIN)-windows-amd64.exe  $(PKG)

clean:
	rm -f $(BIN)
	rm -rf $(DIST)
