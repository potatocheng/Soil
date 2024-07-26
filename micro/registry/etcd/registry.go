package etcd

import (
	"Soil/micro/registry"
	"context"
	"encoding/json"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"sync"
)

var _ registry.Registry = &Registry{}

type Registry struct {
	c       *clientv3.Client
	sess    *concurrency.Session
	cancels []func()
	mutex   sync.Mutex
}

func (r *Registry) Registry(ctx context.Context, si registry.ServiceInstance) error {
	data, err := json.Marshal(si)
	if err != nil {
		return err
	}
	_, err = r.c.Put(ctx, r.instanceKey(si), string(data), clientv3.WithLease(r.sess.Lease()))
	return err
}

func (r *Registry) UnRegistry(ctx context.Context, si registry.ServiceInstance) error {
	_, err := r.c.Delete(ctx, r.instanceKey(si))
	return err
}

func (r *Registry) ListServices(ctx context.Context, serviceName string, opts ...clientv3.OpOption) ([]registry.ServiceInstance, error) {
	resp, err := r.c.Get(ctx, r.serviceKey(serviceName), opts...)
	if err != nil {
		return nil, err
	}

	servInstances := make([]registry.ServiceInstance, 0, len(resp.Kvs))
	for _, pair := range resp.Kvs {
		var si registry.ServiceInstance
		err = json.Unmarshal(pair.Value, &si)
		if err != nil {
			return nil, err
		}
		servInstances = append(servInstances, si)
	}

	return servInstances, nil
}

func (r *Registry) Subscribe(serviceName string) (<-chan registry.Event, error) {
	// 这里设置cancelCtx是因为Watch传入的上下文是 context.Background返回的 WatchChan 不会关闭
	ctx, cancel := context.WithCancel(context.Background())
	r.mutex.Lock()
	r.cancels = append(r.cancels, cancel)
	r.mutex.Unlock()
	ctx = clientv3.WithRequireLeader(ctx)
	watchChan := r.c.Watch(ctx, r.serviceKey(serviceName), clientv3.WithPrefix())
	regEvChan := make(chan registry.Event)
	go func() {
		for {
			select {
			case resp := <-watchChan:
				if resp.Err() != nil {
					continue
				}
				if resp.Canceled {
					return
				}
				for _, ev := range resp.Events {
					regEvChan <- registry.Event{Type: registry.Type(ev.Type)}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return regEvChan, nil
}

func NewRegistry(c *clientv3.Client, opts ...concurrency.SessionOption) (*Registry, error) {
	// session管理租约，续约
	sess, err := concurrency.NewSession(c, opts...)
	if err != nil {
		return nil, err
	}

	return &Registry{
		c:    c,
		sess: sess,
	}, nil
}

func (r *Registry) instanceKey(si registry.ServiceInstance) string {
	return fmt.Sprintf("/micro/%s/%s", si.Name, si.Address)
}

func (r *Registry) serviceKey(sn string) string {
	return fmt.Sprintf("/micro/%s", sn)
}

func (r *Registry) Close() error {
	r.mutex.Lock()
	cancels := r.cancels
	r.cancels = nil
	r.mutex.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	err := r.sess.Close()
	return err
}
