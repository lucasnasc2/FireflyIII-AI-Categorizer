FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /firefly-ai-categorize ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && mkdir -p /data
WORKDIR /app
COPY --from=builder /firefly-ai-categorize .
COPY public/ public/
ENV CONFIG_FILE=/data/config.json
VOLUME /data
EXPOSE 3000
# Same probe docker-compose.yml uses, kept in the image so plain `docker run`
# gets it too. The alpine runtime ships BusyBox wget but no curl/node.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:3000/health || exit 1
CMD ["/app/firefly-ai-categorize"]
