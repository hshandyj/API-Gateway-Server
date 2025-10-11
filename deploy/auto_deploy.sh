#!/bin/bash

# 获取版本号，如果没有设置则使用默认值
VERSION=${VERSION:-"latest"}

echo "使用版本号: $VERSION"

echo "部署Kubernetes..."

# 使用sed替换YAML文件中的版本号
sed -i "s|heshihao0720/hpcp-p-api-gateway-server:.*|heshihao0720/hpcp-p-api-gateway-server:$VERSION|g" api_gateway_server_k8s.yaml

# 替换Kubernetes YAML中的环境变量占位符
sed -i "s|\${KAFKA_BROKERS}|$KAFKA_BROKERS|g" api_gateway_server_k8s.yaml
sed -i "s|\${KAFKA_USERNANME}|$KAFKA_USERNANME|g" api_gateway_server_k8s.yaml
sed -i "s|\${KAFKA_PASSWORD}|$KAFKA_PASSWORD|g" api_gateway_server_k8s.yaml

# 应用Kubernetes配置
minikube kubectl -- apply -f api_gateway_server_k8s.yaml

echo "部署完成！版本: $VERSION"
