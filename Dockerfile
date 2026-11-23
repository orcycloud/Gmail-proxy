# Go 多阶段构建，产物约 8-10MB
FROM golang:1.21-alpine AS builder

WORKDIR /src

# 先拷 go.mod/go.sum，再 go mod tidy 确保 go.sum 存在
# （即使没有外部依赖也会写一份最小 go.sum，避免后续 COPY 变更多次 cache miss）
COPY go.mod go.sum* ./
RUN go mod tidy

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /out/gmail-proxy ./cmd/proxy

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/gmail-proxy /usr/local/bin/gmail-proxy
COPY configs/config.yaml /etc/gmail-proxy/config.yaml

EXPOSE 995/tcp 465/tcp

ENTRYPOINT ["/usr/local/bin/gmail-proxy"]
CMD ["-config", "/etc/gmail-proxy/config.yaml"]
