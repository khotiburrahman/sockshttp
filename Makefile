.PHONY: all tidy vet build arm64 amd64 clean

ARCH ?= arm64
OUT  ?= dist/sockshttp-linux-$(ARCH)

all: tidy vet build

tidy:
	go mod tidy

vet:
	go vet ./...

build:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) \
		go build -trimpath -ldflags="-s -w" -o $(OUT) .

arm64: ; $(MAKE) build ARCH=arm64
amd64: ; $(MAKE) build ARCH=amd64

clean:
	rm -rf dist