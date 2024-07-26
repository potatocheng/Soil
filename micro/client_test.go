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

func TestClient_Get(t *testing.T) {
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	require.NoError(t, err)

	resp, err := etcdClient.Get(context.Background(), "/micro/user-service", clientv3.WithPrefix())
	require.NoError(t, err)
	t.Log(resp)
}

func TestClient_WithRegistry(t *testing.T) {
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	require.NoError(t, err)

	reg, err := etcd.NewRegistry(etcdClient)
	defer reg.Close()
	require.NoError(t, err)

	client, err := NewClient(ClientWithRegistry(reg, time.Minute))
	require.NoError(t, err)

	conn, err := client.Dial(context.Background(), "user-service")
	require.NoError(t, err)

	uc := gen.NewUserClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	resp, err := uc.GetUserById(ctx, &gen.GetUserByIdReq{Id: 123})
	cancel()
	require.NoError(t, err)
	t.Log(resp)
}
