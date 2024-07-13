package proto

import (
	"Soil/micro/proto/gen"
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"net"
	"testing"
)

type Server struct {
	gen.UnimplementedUserServer
}

func (s *Server) GetUserById(ctx context.Context, req *gen.GetUserByIdReq) (*gen.GetUserByIdReply, error) {
	fmt.Println(req)
	return &gen.GetUserByIdReply{
		Msg: "Hello, World",
	}, nil
}

func TestUserServer(t *testing.T) {
	server := grpc.NewServer()
	gen.RegisterUserServer(server, &Server{})
	lis, err := net.Listen("tcp", "127.0.0.1:8087")
	require.NoError(t, err)
	err = server.Serve(lis)
	require.NoError(t, err)
}
