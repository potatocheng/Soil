package round_robin

import (
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"math/rand/v2"
	"sync/atomic"
)

const Name = "round_robin"

type BalanceBuilder struct {
}

func (b *BalanceBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	connections := make([]balancer.SubConn, 0, len(info.ReadySCs))
	for subConn, _ := range info.ReadySCs {
		connections = append(connections, subConn)
	}

	return &Balancer{
		connections: connections,
		next:        rand.Uint64N(uint64(len(connections))),
	}
}

type Balancer struct {
	connections []balancer.SubConn
	next        uint64
}

func (b *Balancer) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	subConn := b.connections[b.next%uint64(len(b.connections))]
	atomic.AddUint64(&b.next, 1)

	return balancer.PickResult{
		SubConn: subConn,
	}, nil
}
