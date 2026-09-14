ARG GO_VERSION=1.27.0
FROM golang:${GO_VERSION} AS builder

WORKDIR /src
COPY src/go.mod src/go.sum ./
RUN go mod download
COPY src/ ./
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -buildvcs=false -tags "netgo,osusergo" \
    -ldflags="-s -w" -o /tungo main.go
WORKDIR /fixture
COPY .github/tests/tunnel/harness/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false \
    -ldflags="-s -w" -o /tungo-e2e .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    iptables iproute2 procps ca-certificates curl iputils-ping \
 && rm -rf /var/lib/apt/lists/*

COPY --from=builder /tungo /usr/local/bin/tungo
COPY --from=builder /tungo-e2e /usr/local/bin/tungo-e2e

ENTRYPOINT []
CMD ["sleep", "infinity"]
