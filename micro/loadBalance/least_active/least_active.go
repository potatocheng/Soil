package least_active

import (
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"math"
	"sync/atomic"
)

type BalancerBuilder struct {
}

func (b *BalancerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	connections := make([]*activeConn, 0, len(info.ReadySCs))
	for sc, _ := range info.ReadySCs {
		connections = append(connections, &activeConn{
			conn: sc,
		})
	}

	return &Balancer{
		connections: connections,
	}
}

type Balancer struct {
	connections []*activeConn
}

func (b *Balancer) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	// 在这里对各个连接数都是采用原子操作，这样可能会导致计数没有加锁这么精确，不过这样并发度更高
	res := &activeConn{
		cnt: math.MaxUint32,
	}

	for _, sc := range b.connections {
		if atomic.LoadUint32(&res.cnt) > sc.cnt {
			res = sc
		}
	}

	atomic.AddUint32(&res.cnt, 1)

	return balancer.PickResult{
		SubConn: res.conn,
		Done: func(info balancer.DoneInfo) {
			atomic.AddUint32(&res.cnt, -1)
		},
	}, nil
}

type activeConn struct {
	conn balancer.SubConn
	cnt  uint32
}
