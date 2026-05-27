.PHONY: build test test-short vet run clean docs lint

BINARY  := suanime
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS := -X main.commit=$(COMMIT)

build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

run: build
	./$(BINARY)

test:
	go test ./... -count=1 -timeout 120s

test-short:
	go test ./... -short -count=1

vet:
	go vet ./...

vet-shadow:
	go vet -vettool=$$(which shadow) ./... 2>/dev/null || go vet ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)" || (echo "unformatted files:" && gofmt -l . && exit 1)

fmt:
	gofmt -w .

clean:
	rm -f $(BINARY)

docs:
	@echo "docs/README.md       — overview, features, quick start"
	@echo "docs/CLI.md           — CLI command reference"
	@echo "docs/CONFIG.md        — config file documentation"
	@echo "docs/ARCHITECTURE.md  — code structure, data flow"
	@echo "docs/COVERART.md      — cover art feature plan"

all: vet build test-short

install: build
	install -Dm755 $(BINARY) $(HOME)/.local/bin/$(BINARY)

uninstall:
	rm -f $(HOME)/.local/bin/$(BINARY)

release: vet test
	go build -ldflags="-s -w $(LDFLAGS)" -o $(BINARY) .

.PHONY: commit push
commit: build vet test-short
	git add -A
	git commit

push:
	git push
