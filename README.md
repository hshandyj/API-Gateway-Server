# HPCP API Gateway Server

高性能协作平台的API网关服务，使用Go语言和Gin框架构建，提供统一的API入口和服务路由功能。

## 架构设计

### 整体架构
```
外部请求 → API网关(Gin) 
           ├── gRPC → 用户鉴权服务 (同步认证)
           ├── WebSocket代理 → 文档服务 (实时协作)
           └── HTTP → Kafka → Python分析服务 (异步数据收集)
```

### 技术栈
- **Web框架**: Gin (高性能HTTP框架)
- **通信协议**: gRPC, WebSocket, Kafka
- **配置管理**: 环境变量
- **日志系统**: Logrus (结构化日志)
- **容器化**: Docker

### 服务通信设计

#### gRPC使用场景
- **API网关 ↔ 用户鉴权验证服务**: 同步认证和权限检查
- **服务间配置同步**: 服务发现和健康检查
- **高频调用场景**: 利用二进制序列化提升性能

#### WebSocket代理
- **实时文档协作**: 代理客户端到文档服务的WebSocket连接
- **用户会话管理**: 在网关层处理用户认证和会话
- **消息过滤和日志**: 可在代理层添加消息处理逻辑

#### Kafka异步处理
- **用户行为分析**: 收集用户操作数据发送到Python分析服务
- **削峰填谷**: 处理高并发的数据收集请求
- **事件驱动**: 支持微服务间的异步通信

## 项目结构

```
├── main.go                 # 主入口文件
├── go.mod                  # Go模块依赖
├── Dockerfile             # Docker构建文件
├── config.env.example     # 环境变量配置示例
├── internal/              # 内部包
│   ├── config/           # 配置管理
│   ├── grpc/             # gRPC客户端
│   ├── kafka/            # Kafka生产者
│   ├── websocket/        # WebSocket代理
│   ├── middleware/       # 中间件
│   └── handlers/         # HTTP处理器
├── proto/                # Protocol Buffers定义
└── deploy/               # 部署相关文件
```

## 分层架构

### 1. 网关层 (Gateway Layer)
- 统一入口和路由
- 请求验证和预处理
- 响应聚合和后处理

### 2. 中间件层 (Middleware Layer)
- **认证中间件**: JWT token验证
- **权限中间件**: 基于RBAC的权限检查
- **分析中间件**: 用户行为数据收集
- **日志中间件**: 请求/响应日志记录

### 3. 服务代理层 (Proxy Layer)
- **gRPC代理**: 同步服务调用
- **WebSocket代理**: 实时连接代理
- **HTTP代理**: RESTful API转发

### 4. 数据处理层 (Data Layer)
- **Kafka生产者**: 异步消息发送
- **缓存处理**: (预留扩展)
- **数据转换**: 请求/响应格式转换

## 快速开始

### 前置要求
- Go 1.23+
- Protocol Buffers编译器
- Docker (可选)

### 安装开发工具
```bash
# 安装Protocol Buffers编译器
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### 安装依赖
```bash
go mod tidy
go mod download
```

### 生成protobuf文件
```bash
# 创建proto目录
mkdir -p proto/auth

# 生成protobuf文件
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/auth.proto
```

### 配置环境变量
```bash
cp config.env.example .env
# 编辑.env文件设置你的配置
```

### 开发模式运行
```bash
# 设置Gin为调试模式并运行
GO_ENV=debug
GIN_MODE=debug
go run main.go
```

### 生产模式构建和运行
```bash
# 构建项目
go build -o bin/api-gateway main.go

# 运行项目
./bin/api-gateway
```

### 完整开发流程
```bash
# 1. 首次设置
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 2. 安装依赖
go mod tidy
go mod download

# 3. 生成protobuf文件
mkdir -p proto/auth
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/auth.proto

# 4. 配置环境变量
cp config.env.example .env
# 编辑.env文件

# 5. 开发模式运行
GIN_MODE=debug go run main.go
```

### 清理构建文件
```bash
# 删除构建文件
rm -rf bin/
rm -rf proto/auth/*.pb.go
```

## API接口

### 健康检查
```bash
GET /health        # 基础健康检查
GET /ready         # 就绪状态检查
```

### 认证相关
```bash
# 所有/api/v1下的接口都需要Bearer Token认证
Authorization: Bearer <your-jwt-token>
```

### 文档服务
```bash
GET    /api/v1/documents     # 获取文档列表
GET    /api/v1/documents/:id # 获取文档详情
POST   /api/v1/documents     # 创建文档
PUT    /api/v1/documents/:id # 更新文档
DELETE /api/v1/documents/:id # 删除文档
```

### WebSocket连接
```bash
GET /api/v1/ws/document      # WebSocket文档协作连接
```

### 用户信息
```bash
GET /api/v1/users/me         # 获取当前用户信息
```

## 环境变量配置

| 变量名 | 描述 | 默认值 |
|--------|------|--------|
| SERVER_PORT | 服务器端口 | 8080 |
| AUTH_SERVICE_ADDR | 认证服务地址 | localhost:50051 |
| KAFKA_BROKERS | Kafka代理地址 | localhost:9092 |
| KAFKA_TOPIC_ANALYTICS | 用户分析主题 | user-analytics |
| DOCUMENT_SERVICE_ADDR | 文档服务WebSocket地址 | ws://localhost:8081/ws |

## Docker部署

### 构建镜像
```bash
docker build -t hpcp-api-gateway:latest .
```

### 运行容器
```bash
docker run -p 8080:8080 \
  -e AUTH_SERVICE_ADDR=your-auth-service:50051 \
  -e KAFKA_BROKERS=your-kafka:9092 \
  hpcp-api-gateway:latest
```

## 开发指南

### 代码规范
```bash
# 格式化代码
go fmt ./...

# 代码检查
golangci-lint run

# 运行测试
go test -v ./...
```

### 添加新的服务代理
1. 在`internal/`下创建新的包
2. 实现服务客户端
3. 在`main.go`中注册路由
4. 添加相应的中间件

### 扩展中间件
1. 在`internal/middleware/`下创建新文件
2. 实现`gin.HandlerFunc`接口
3. 在路由中使用中间件

## 监控和日志

### 日志格式
项目使用结构化JSON日志格式，包含：
- 请求ID
- 用户ID
- 响应时间
- 状态码
- 错误信息

### 监控指标
- HTTP请求响应时间
- gRPC调用成功率
- Kafka消息发送状态
- WebSocket连接数量

## 故障排查

### 常见问题
1. **gRPC连接失败**: 检查认证服务是否启动
2. **Kafka发送失败**: 检查Kafka集群连接
3. **WebSocket代理失败**: 检查文档服务地址配置

### 日志查看
```bash
# 实时查看日志
docker logs -f <container-id>

# 过滤错误日志
docker logs <container-id> 2>&1 | grep ERROR
```

## 性能优化建议

1. **连接池管理**: gRPC和HTTP客户端使用连接池
2. **缓存策略**: 对频繁访问的数据进行缓存
3. **异步处理**: 使用Kafka处理非关键路径的操作
4. **监控告警**: 设置关键指标的监控和告警

## 许可证

MIT License
