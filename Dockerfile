# Build the static binary.
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /handlecheckercli ./cmd/handlecheckercli

# Runtime image with espeak-ng, which enables the phoneme-level sound check.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends espeak-ng \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /handlecheckercli /usr/local/bin/handlecheckercli
ENTRYPOINT ["handlecheckercli"]
