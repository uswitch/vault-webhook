APP  = vault-webhook

ARCH = amd64
BIN  = bin/$(APP)
BIN_LINUX  = $(BIN)-linux-$(ARCH)
BIN_DARWIN = $(BIN)-darwin-$(ARCH)
IMAGE   = localhost/$(APP)

SOURCES = $(shell find . -type f -iname "*.go")

.PHONY: all build vet fmt test run image clean private

all: test build

$(BIN_DARWIN): $(SOURCES)
	GOARCH=$(ARCH) GOOS=darwin go build -o $(BIN_DARWIN)

$(BIN_LINUX): $(SOURCES)
	GOARCH=$(ARCH) GOOS=linux CGO_ENABLED=0 go build -o $(BIN_LINUX)

build: $(BIN_DARWIN) $(BIN_LINUX) fmt vet

vet:
	go vet ./...

fmt:
	go fmt ./...

test: fmt vet
	go test ./... -coverprofile cover.out

image: Dockerfile $(BIN_LINUX)
	docker image build -t $(IMAGE) .

run-image: image
	docker run --rm -ti $(IMAGE)

clean:
	rm -rf bin/
