# 后端 API 多阶段构建
# 说明：默认 CGO_ENABLED=0（镜像更小、无 glibc 依赖）。若部署时要用 SQLite
# 而非 PostgreSQL，必须改为 CGO_ENABLED=1 并把运行阶段换成 debian:bookworm-slim。
FROM golang:1.24-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

# 日历提醒依赖本地时区，部署时必须注入 TZ=Asia/Shanghai，否则提醒会错 8 小时
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /out/api /app/api

EXPOSE 8080

ENTRYPOINT ["/app/api"]
