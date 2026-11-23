# Go 多阶段构建，产物约 8-10MB
FROM golang:1.21-alpine AS builder
WORKDIR /src

RUN apk add --no-cache git

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/gmail-proxy ./cmd/proxy

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/gmail-proxy /usr/local/bin/gmail-proxy
COPY configs/config.yaml /etc/gmail-proxy/config.yaml

EXPOSE 995/tcp 465/tcp

ENTRYPOINT ["/usr/local/bin/gmail-proxy"]
CMD ["-config", "/etc/gmail-proxy/config.yaml"]
