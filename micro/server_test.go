package micro

import (
	"Soil/micro/proto/gen"
	"Soil/micro/registry/etcd"
	"context"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
	"testing"
	"time"
)

func TestServer(t *testing.T) {
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	require.NoError(t, err)
	r, err := etcd.NewRegistry(etcdClient)
	require.NoError(t, err)
	server, err := NewServer("user-service", time.Minute, ServerWithRegistry(r))
	require.NoError(t, err)
	us := &UserServiceServer{}
	gen.RegisterUserServer(server, us)

	err = server.Start("127.0.0.1:8081")
	t.Log(err)
}

type UserServiceServer struct {
	gen.UnimplementedUserServer
}

func (u *UserServiceServer) GetUserById(ctx context.Context, request *gen.GetUserByIdReq) (*gen.GetUserByIdReply, error) {
	return &gen.GetUserByIdReply{
		Msg: "Hello, World",
	}, nil
}
