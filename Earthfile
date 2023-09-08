VERSION 0.7
FROM golang:1.21-bookworm
WORKDIR /app

docker-all:
  BUILD --platform=linux/amd64 --platform=linux/arm64 +docker

docker:
  ARG TARGETARCH
  ARG VERSION
  FROM debian:bookworm-slim
  COPY LICENSE /usr/local/share/devsyncer/
  COPY (+devsyncer/devsyncer --GOARCH=${TARGETARCH}) /usr/local/bin/
  EXPOSE 8080/tcp
  EXPOSE 8443/tcp
  ENTRYPOINT ["/usr/local/bin/devsyncer"]
  SAVE IMAGE --push ghcr.io/gpu-ninja/devsyncer:${VERSION}
  SAVE IMAGE --push ghcr.io/gpu-ninja/devsyncer:latest

devsyncer:
  ARG GOOS=linux
  ARG GOARCH=amd64
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN CGO_ENABLED=0 go build --ldflags '-s' -o devsyncer cmd/devsyncer/main.go
  SAVE ARTIFACT ./devsyncer AS LOCAL dist/devsyncer-${GOOS}-${GOARCH}

tidy:
  LOCALLY
  RUN go mod tidy
  RUN go fmt ./...

lint:
  FROM golangci/golangci-lint:v1.54.2
  WORKDIR /app
  COPY . ./
  RUN golangci-lint run --timeout 5m ./...

test:
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN go test -coverprofile=coverage.out -v ./...
  SAVE ARTIFACT ./coverage.out AS LOCAL coverage.out