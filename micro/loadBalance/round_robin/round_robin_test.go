package round_robin

import (
	"Soil/micro"
	"Soil/micro/proto/gen"
	"Soil/micro/registry/etcd"
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"testing"
	"time"
)

func TestBalancer_e2e_RoundRobin(t *testing.T) {
	// 创建一个连接到etcd的连接
	endpoint, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	require.NoError(t, err)
	// 管理连接到etcd的连接
	registry, err := etcd.NewRegistry(endpoint)
	require.NoError(t, err)
	// 创建服务器
	for i := 0; i < 3; i++ {
		go func() {
			server, err := micro.NewServer("ping", time.Minute, micro.ServerWithRegistry(registry))
			defer server.Close()
			require.NoError(t, err)
			addr := fmt.Sprintf("127.0.0.1:2009%d", i)
			is := &InteractionServer{
				address: addr,
			}
			gen.RegisterInteractionServer(server, is)
			err = server.Start(addr)
			require.NoError(t, err)
		}()
	}

	time.Sleep(time.Second * 3)

	// 注册balancer
	balancer.Register(base.NewBalancerBuilder(Name, &BalanceBuilder{}, base.Config{HealthCheck: true}))

	// 创建客户端
	require.NoError(t, err)
	client, err := micro.NewClient(micro.ClientWithRegistry(registry, time.Minute))
	require.NoError(t, err)
	conn, err := client.Dial(context.Background(), "ping", grpc.WithDefaultServiceConfig(`{"LoadBalancingPolicy": "round_robin"}`))
	require.NoError(t, err)
	defer conn.Close()
	for i := 0; i < 3; i++ {
		ic := gen.NewInteractionClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		resp, err := ic.Ping(ctx, &gen.Ping{Msg: "call"})
		require.NoError(t, err)
		fmt.Println(resp)
	}
}

type InteractionServer struct {
	address string
	gen.UnimplementedInteractionServer
}

func (i InteractionServer) Ping(ctx context.Context, ping *gen.Ping) (*gen.Pong, error) {
	return &gen.Pong{
		Msg: i.address,
	}, nil
}
