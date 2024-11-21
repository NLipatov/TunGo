# syntax=docker/dockerfile:1

FROM golang:latest

# Set destination for COPY
WORKDIR /src

# Copy files
COPY . ./

# Download dependencies
RUN go mod download
RUN apt-get update && apt-get install -y iproute2 iptables

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o server-binary main.go
RUN chmod +x server-binary

# Run
CMD ["./server-binary", "s"]