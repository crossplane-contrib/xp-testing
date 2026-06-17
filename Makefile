
.PHONY: lint
lint:
	golangci-lint run ./...


.PHONY: test
test:
	go test -coverprofile cover.out  -v ./...

.PHONY: e2e
e2e:
	go test -v ./test/e2e/... -tags=e2e -count=1 -test.v

.PHONY: upgrade
upgrade:
	go test -v ./test/upgrade/... -tags=upgrade -count=1 -test.v

.PHONY: build
build:
	go build -v ./...

.PHONY: mod
mod:
	go mod tidy
.PHONY: all
all: mod lint build test e2e upgrade

