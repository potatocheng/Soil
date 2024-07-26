package registry

import (
	"context"
	clientv3 "go.etcd.io/etcd/client/v3"
	"io"
)

type Registry interface {
	Registry(ctx context.Context, si ServiceInstance) error
	UnRegistry(ctx context.Context, si ServiceInstance) error

	ListServices(ctx context.Context, serviceName string, opts ...clientv3.OpOption) ([]ServiceInstance, error)
	Subscribe(serviceName string) (<-chan Event, error)

	io.Closer
}

type ServiceInstance struct {
	// 服务名
	Name string
	// ip + port
	Address string

	Weight uint32
	Group  string
}

type Type int32

const (
	PUT    Type = 0
	DELETE Type = 1
)

type Event struct {
	Type Type
}
