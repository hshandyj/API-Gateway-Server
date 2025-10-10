#!/bin/bash

# 获取版本号，如果没有设置则使用默认值
VERSION=${VERSION:-"latest"}

echo "使用版本号: $VERSION"

echo "部署Kubernetes..."

# 检查并创建Docker Hub Secret（如果不存在）
if ! minikube kubectl -- get secret dockerhub-secret &> /dev/null; then
    echo "创建Docker Hub Secret..."
    echo "请确保在服务器上已设置以下环境变量："
    echo "  DOCKER_USERNAME"
    echo "  DOCKER_PASSWORD"
    echo "  DOCKER_EMAIL"
    
    if [ -n "$DOCKER_USERNAME" ] && [ -n "$DOCKER_PASSWORD" ] && [ -n "$DOCKER_EMAIL" ]; then
        minikube kubectl -- create secret docker-registry dockerhub-secret \
            --docker-server=https://index.docker.io/v1/ \
            --docker-username="$DOCKER_USERNAME" \
            --docker-password="$DOCKER_PASSWORD" \
            --docker-email="$DOCKER_EMAIL"
        echo "Secret创建成功"
    else
        echo "警告: Docker凭证环境变量未设置，如果使用私有镜像可能会失败"
    fi
else
    echo "Docker Hub Secret已存在，跳过创建"
fi

# 使用sed替换YAML文件中的版本号
sed -i "s|heshihao0720/hpcp-p-api-gateway-server:.*|heshihao0720/hpcp-p-api-gateway-server:$VERSION|g" api_gateway_server_k8s.yaml

# 应用Kubernetes配置
minikube kubectl -- apply -f api_gateway_server_k8s.yaml

echo "部署完成！版本: $VERSION"
