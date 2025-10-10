# 构建阶段
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并构建
COPY . .
RUN go build -o api-gateway main.go

# 运行阶段
FROM alpine:latest

# 安装必要的包
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# 从构建阶段复制二进制文件和配置
COPY --from=builder /app/api-gateway .
COPY --from=builder /app/internal/config/APIGatewayConfig.yaml ./internal/config/

EXPOSE 8080

# 设置环境变量
ENV GIN_MODE=release
ENV GO_ENV=production

CMD ["./api-gateway"]