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

run-docker: docker-build
	mkdir -p localfiles/docker-base-dir
	docker rm gofunc || true
	docker run --rm -ti \
		-p 9000:9000 \
		-e BASE_DIR=/var/gofunc \
		-v $(PWD)/localfiles/docker-base-dir:/var/gofunc \
		--name gofunc \
		andrebq/gofunc:latest

upload: build
	[[ -n "$(name)" ]] || { echo "make name=<name> is required"; exit 1; }
	[[ -n "$(srcDir)" ]] || { echo "make srcDir=<src dir> is required"; exit 1; }
	$(PWD)/dist/gofunc upload --dir $(srcDir) --name $(name)