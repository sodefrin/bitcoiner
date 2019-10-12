PACKAGES = $(shell go list ./...)
VERSION = $(shell git rev-parse --verify HEAD)

.PHONY: build
build:
	go build -o bitcoiner -ldflags "-X github.com/sodefrin/bitcoiner/config.Version=${VERSION}" -mod=readonly main.go

.PHONY: install
install:
	mv bitcoiner /usr/local/bin/
