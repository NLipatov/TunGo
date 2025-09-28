# syntax=docker/dockerfile:1

# Stage 1: build tungo
FROM golang:1.25 AS builder

# Set destination for COPY
WORKDIR /src

# Copy files
COPY . ./

# Download dependencies
RUN go mod download

RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build \
    -trimpath -buildvcs=false -tags "netgo,osusergo" \
    -ldflags="-s -w" \
    -o tungo main.go

# Stage 2: runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    iptables iproute2 procps ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /src/tungo /usr/local/bin/tungo

ENTRYPOINT ["/usr/local/bin/tungo"]
CMD ["s"]
