package micro

import (
	"Soil/micro/registry"
	"context"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
	"time"
)

type grpcResolverBuilder struct {
	r       registry.Registry
	timeout time.Duration
}

func NewResolverBuilder(r registry.Registry, timeout time.Duration) (resolver.Builder, error) {
	return &grpcResolverBuilder{
		r:       r,
		timeout: timeout,
	}, nil
}

func (g *grpcResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	res := &grpcResolver{
		target:  target,
		cc:      cc,
		r:       g.r,
		timeout: g.timeout,
		close:   make(chan struct{}),
	}
	res.resolve()
	go res.watch()
	return res, nil
}

func (g *grpcResolverBuilder) Scheme() string {
	return "registry"
}

type grpcResolver struct {
	target resolver.Target
	// ClientConn 接口由 gRPC 框架实现，解析器（resolver）通过这个接口与 gRPC 客户端通信，通知客户端解析状态的变化、报告错误、以及解析后的地址列表等信息。
	cc resolver.ClientConn

	r       registry.Registry
	timeout time.Duration
	close   chan struct{}
}

// ResolveNow  当 gRPC 客户端希望重新解析目标服务名称时调用这个方法。它是一个提示，Resolver 可以选择忽略这个提示，如果认为没有必要重新解析的话。
func (r *grpcResolver) ResolveNow(options resolver.ResolveNowOptions) {
	r.resolve()
}

func (r *grpcResolver) resolve() {
	// 从服务中心找到实例
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	serviceInstances, err := r.r.ListServices(ctx, r.target.Endpoint(), clientv3.WithPrefix())
	if err != nil {
		r.cc.ReportError(err)
		return
	}

	addressList := make([]resolver.Address, 0, len(serviceInstances))
	for _, serviceInstance := range serviceInstances {
		addressList = append(addressList, resolver.Address{
			Addr:       serviceInstance.Address,
			Attributes: attributes.New("weight", serviceInstance.Weight).WithValue("group", serviceInstance.Group),
		})
	}
	// 更新实例到客户端
	err = r.cc.UpdateState(resolver.State{Addresses: addressList})
	if err != nil {
		r.cc.ReportError(err)
		return
	}
}

func (r *grpcResolver) watch() {
	evChan, err := r.r.Subscribe(r.target.Endpoint())
	if err != nil {
		r.cc.ReportError(err)
		return
	}
	// 监听到有事件发生，表示注册中心中服务实例发生了变化，需要重新获取服务列表
	select {
	case <-evChan:
		r.resolve()
	case <-r.close:
		return
	}

}

func (r *grpcResolver) Close() {
	close(r.close)
}
