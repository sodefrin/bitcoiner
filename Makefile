PACKAGES = $(shell go list ./...)
VERSION = $(shell git rev-parse --verify HEAD)

.PHONY: build
build:
	go build -o bitcoiner -mod=readonly main.go

.PHONY: install
install:
	mv bitcoiner /usr/local/bin/

.PHONY: lint
lint:
	make vet
	make staticcheck
	make errcheck

.PHONY: vet
vet:
	go vet $(PACKAGES)

.PHONY: staticcheck
staticcheck:
	staticcheck $(PACKAGES)

.PHONY: errcheck
errcheck:
	errcheck $(PACKAGES)
