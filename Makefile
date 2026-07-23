.PHONY: check build vet test

check: build vet test

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./...
