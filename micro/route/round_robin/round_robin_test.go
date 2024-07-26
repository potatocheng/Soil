package round_robin

import (
	"Soil/micro/registry/etcd"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"sync"
	"testing"
)

func TestRoute_RR(t *testing.T) {
	// 创建服务端
	// 创建etcd连接
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	require.NoError(t, err)
	m := sync.Mutex{}
	m.Lock()
	m.Unlock()
	// 创建registry, 自动管理etcd连接
	registry, err := etcd.NewRegistry(etcdClient, concurrency.WithTTL(60))
	require.NoError(t, err)

	// 创建客户端
}
