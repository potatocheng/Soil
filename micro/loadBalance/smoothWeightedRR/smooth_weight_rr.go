package smoothWeightedRR

import (
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"math"
	"sync"
)

type BalanceBuilder struct {
}

func (b *BalanceBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	connections := make([]*weightConn, 0, len(info.ReadySCs))
	for sc, connInfo := range info.ReadySCs {
		weight := connInfo.Address.Attributes.Value("weight").(uint32)
		connections = append(connections, &weightConn{
			sc:              sc,
			efficientWeight: weight,
			currentWeight:   weight,
		})
	}

	return &Balancer{connections: connections}
}

type Balancer struct {
	connections []*weightConn
	mutex       sync.Mutex
}

func (b *Balancer) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	if len(b.connections) == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	var totalWeight uint32
	var res *weightConn
	for _, c := range b.connections {
		c.mutex.Lock()
		totalWeight += c.efficientWeight
		c.currentWeight += c.efficientWeight
		if res == nil || res.currentWeight < c.currentWeight {
			res = c
		}
		c.mutex.Unlock()
	}
	if res == nil {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	res.mutex.Lock()
	res.currentWeight -= totalWeight
	res.mutex.Unlock()
	return balancer.PickResult{
		SubConn: res.sc,
		Done: func(info balancer.DoneInfo) {
			res.mutex.Lock()
			if info.Err != nil && res.efficientWeight == 0 {
				return
			}
			if info.Err == nil && res.efficientWeight == math.MaxUint32 {
				return
			}
			if info.Err != nil {
				res.efficientWeight--
			} else {
				res.efficientWeight++
			}
			res.mutex.Unlock()
		},
	}, nil
}

type weightConn struct {
	sc              balancer.SubConn
	efficientWeight uint32
	currentWeight   uint32
	mutex           sync.Mutex
}
