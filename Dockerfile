FROM golang:1-alpine AS builder
WORKDIR /app
ADD go.mod go.sum ./
RUN go mod download
ADD ./ ./
RUN go build -o gofunc ./cmd/gofunc


FROM golang:1-alpine
COPY --from=builder /app/gofunc /usr/local/bin/
ENV BASE_DIR=/var/gofunc
ENV BIND_PORT=9000
ENV BIND_ADDR=0.0.0.0
CMD [ "gofunc", "serve" ]