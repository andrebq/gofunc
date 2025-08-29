.PHONY: build default test run

default: test build

test:
	go test ./...

build:
	mkdir -p dist
	go build -o dist/gofunc ./cmd/gofunc

docker-build:
	docker build -t andrebq/gofunc:latest .

run: build
	mkdir -p $(PWD)/localfiles/server
	$(PWD)/dist/gofunc serve --base-dir $(PWD)/localfiles/server

upload: build
	[[ -n "$(name)" ]] || { echo "make name=<name> is required"; exit 1; }
	[[ -n "$(srcDir)" ]] || { echo "make srcDir=<src dir> is required"; exit 1; }
	$(PWD)/dist/gofunc upload --dir $(srcDir) --name $(name)