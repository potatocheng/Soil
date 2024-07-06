package rpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestClientAndServer(t *testing.T) {
	server := NewServer("tcp", "127.0.0.1:8080")

	server.RegisterService(&UserServiceServer{})
	go func() {
		err := server.Start()
		require.NoError(t, err)
	}()

	time.Sleep(time.Second * 3)

	clientUserService := &UserService{}
	err := InitClientProxy("127.0.0.1:8080", clientUserService)
	require.NoError(t, err)
	resp, err := clientUserService.GetUserById(context.Background(), &GetUserByIdReq{Id: 123})
	require.NoError(t, err)
	fmt.Println(resp.Msg)
}
