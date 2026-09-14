ARG GO_VERSION=1.27.0
FROM golang:${GO_VERSION} AS builder
WORKDIR /src
COPY src/go.mod src/go.sum ./
RUN go mod download
COPY src/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false \
    -tags "netgo,osusergo" -ldflags="-s -w" -o /tungo main.go
WORKDIR /fixture
COPY .github/tests/tunnel/harness/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false \
    -ldflags="-s -w" -o /tungo-e2e .

FROM alpine:3.23 AS guest
RUN apk add --no-cache linux-virt iproute2 iptables tcpdump
COPY --from=builder /tungo /usr/local/bin/tungo
COPY --from=builder /tungo-e2e /usr/local/bin/tungo-e2e
COPY .github/tests/tunnel/compatibility/server-vm/init-server.sh /init
RUN chmod 755 /init && mkdir -p /proc /sys /dev /run /tmp

FROM alpine:3.23 AS packer
RUN apk add --no-cache cpio gzip
COPY --from=guest / /rootfs/
RUN cp /rootfs/boot/vmlinuz-virt /kernel && rm -rf /rootfs/boot \
    && cd /rootfs && find . -print0 | cpio --null -o -H newc | gzip -1 > /initramfs.gz

FROM scratch
COPY --from=packer /kernel /initramfs.gz /
