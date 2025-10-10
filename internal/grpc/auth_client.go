package grpc

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "HPCP-P/API-Gateway-Server/proto/auth"
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
