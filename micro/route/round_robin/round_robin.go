package round_robin

import (
	"Soil/micro/route"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
	"math/rand/v2"
	"sync/atomic"
)

const Name = "round_robin"

type BalanceBuilder struct {
	filter route.Filter
}

func (b *BalanceBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	connections := make([]attributeConn, 0, len(info.ReadySCs))
	for subConn, scInfo := range info.ReadySCs {
		connections = append(connections, attributeConn{
			conn: subConn,
			addr: scInfo.Address,
		})
	}

	return &Balancer{
		connections: connections,
		next:        rand.Uint64N(uint64(len(connections))),
		filter:      b.filter,
	}
}

type Balancer struct {
	connections []attributeConn
	next        uint64
	filter      route.Filter
}

func (b *Balancer) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	candidates := make([]attributeConn, 0, len(b.connections))
	for _, conn := range b.connections {
		if b.filter != nil && b.filter(info, conn.addr) {
			continue
		}

		candidates = append(candidates, attributeConn{
			conn: conn.conn,
			addr: conn.addr,
		})
	}

	subConn := candidates[b.next%uint64(len(candidates))]
	atomic.AddUint64(&b.next, 1)

	return balancer.PickResult{
		SubConn: subConn.conn,
	}, nil
}

type attributeConn struct {
	conn balancer.SubConn
	addr resolver.Address
}
