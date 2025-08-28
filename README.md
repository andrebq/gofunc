# gofaas

This image accepts a zip of a Go program via PUT to /$admin/recompile, builds it and runs it. It proxies requests to the running program on 127.0.0.1:8000. The repo contains a small CLI to create and upload zip files respecting .gitignore and .gofaas.ignore.

Build image:

    docker build -t gofaas:local .

Run:

    docker run -p 8080:8080 gofaas:local

Upload via CLI:

    go run ./cmd/gofaas -mode=cli -src=./your-app -out=app.zip -url=http://localhost:8080/$admin/recompile

Tests:

    go test ./...
