
.PHONY: lint
lint:
	golangci-lint run ./...


.PHONY: test
test:
	go test -coverprofile cover.out  -v ./...


.PHONY: build
build:
	go build -v ./...

.PHONY: all
all: lint build test

