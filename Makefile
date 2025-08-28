.PHONY: build default test run

default: test build

test:
	go test ./...

build:
	mkdir -p dist
	go build -o dist/gofunc ./cmd/gofunc

run: build
	mkdir -p $(PWD)/localfiles/server
	$(PWD)/dist/gofunc serve --base-dir $(PWD)/localfiles/server