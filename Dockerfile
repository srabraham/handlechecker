# Build the static binaries (CLI + web server).
FROM golang:bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /handlecheckerweb ./cmd/handlecheckerweb

# Runtime image with espeak-ng, which enables the phoneme-level sound check.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends espeak-ng \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /handlecheckerweb /usr/local/bin/handlecheckerweb
# Default to the CLI for backward compatibility; override the entrypoint to run
# the web server, which serves plain HTTP on 8080 (TLS is terminated upstream).
EXPOSE 8080
ENTRYPOINT ["handlecheckerweb"]
