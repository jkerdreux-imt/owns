# Stage 1: Build
FROM golang:1.25-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /owns

# Stage 2: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /owns /usr/local/bin/owns
EXPOSE 53/udp
ENTRYPOINT ["owns"]
CMD ["-bindAddr", "127.0.0.1", "-confDir", "/etc/owns", "-port", "53", "-logLevel", "INFO"]
