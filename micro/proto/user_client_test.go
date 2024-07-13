package proto

import (
	"Soil/micro/proto/gen"
	"context"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"testing"
	"time"
)

func TestGrpcClient(t *testing.T) {
	conn, err := grpc.NewClient("127.0.0.1:8087", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	userClient := gen.NewUserClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	resp, err := userClient.GetUserById(ctx, &gen.GetUserByIdReq{Id: 123})
	cancel()
	require.NoError(t, err)
	t.Log(resp)
}
