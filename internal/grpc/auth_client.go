package grpc

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	pb "APIGatewayServer/proto/auth"
)

type AuthClient struct {
	conn   *grpc.ClientConn
	client pb.AuthServiceClient
}

func NewAuthClient(addr string) (*AuthClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	client := pb.NewAuthServiceClient(conn)

	return &AuthClient{
		conn:   conn,
		client: client,
	}, nil
}

func (ac *AuthClient) Close() error {
	return ac.conn.Close()
}

func (ac *AuthClient) ValidateToken(ctx context.Context, token string) (*pb.ValidateTokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req := &pb.ValidateTokenRequest{
		Token: token,
	}

	return ac.client.ValidateToken(ctx, req)
}

func (ac *AuthClient) GetUserInfo(ctx context.Context, userID string) (*pb.GetUserInfoResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req := &pb.GetUserInfoRequest{
		UserId: userID,
	}

	return ac.client.GetUserInfo(ctx, req)
}

func (ac *AuthClient) CheckPermission(ctx context.Context, userID, resource, action string) (*pb.CheckPermissionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req := &pb.CheckPermissionRequest{
		UserId:   userID,
		Resource: resource,
		Action:   action,
	}

	return ac.client.CheckPermission(ctx, req)
}

// CheckHealth 检查认证服务的健康状态
// 通过检查 gRPC 连接状态来判断服务是否可用，不会产生无效的 token 验证请求
func (ac *AuthClient) CheckHealth(ctx context.Context) error {
	// 检查连接状态
	state := ac.conn.GetState()

	// READY 和 IDLE 状态都表示连接正常
	if state == connectivity.Ready || state == connectivity.Idle {
		return nil
	}

	// 如果连接处于其他状态，尝试等待连接变为 READY
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// 等待连接状态变化
	if ac.conn.WaitForStateChange(ctx, state) {
		newState := ac.conn.GetState()
		if newState == connectivity.Ready || newState == connectivity.Idle {
			return nil
		}
	}

	return grpc.ErrClientConnClosing
}
