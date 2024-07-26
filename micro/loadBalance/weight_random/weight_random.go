package weight_random

import (
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"math/rand/v2"
)

type BalancerBuilder struct {
}

func (b *BalancerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	var totalWeight uint32 = 0
	connections := make([]*weightConn, 0, len(info.ReadySCs))
	for sc, scInfo := range info.ReadySCs {
		weight := scInfo.Address.Attributes.Value("weight").(uint32)
		totalWeight += weight
		connections = append(connections, &weightConn{
			conn:   sc,
			weight: weight,
		})
	}

	return &Balancer{
		connections: connections,
	}
}

type Balancer struct {
	connections []*weightConn
	totalWeight uint32
}

func (b *Balancer) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	randWeight := rand.Uint32N(b.totalWeight)

	for _, sc := range b.connections {
		randWeight -= sc.weight
		if randWeight < 0 {
			return balancer.PickResult{
				SubConn: sc.conn,
				Done:    func(info balancer.DoneInfo) {},
			}, nil
		}
	}

	return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
}

type weightConn struct {
	conn   balancer.SubConn
	weight uint32
}
