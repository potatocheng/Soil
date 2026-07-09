package net

import (
	"math/rand/v2"
	"sync/atomic"
)

// Balancer 在候选 endpoint 中选择一个。candidates 保证非空。
type Balancer interface {
	Pick(candidates []*endpointNode) *endpointNode
}

// BalancerPolicy 内置策略名
type BalancerPolicy string

const (
	// PolicyRoundRobin 轮询（默认）
	PolicyRoundRobin BalancerPolicy = "round_robin"
	// PolicyRandom 随机
	PolicyRandom BalancerPolicy = "random"
	// PolicyLeastActive 最少在途请求
	PolicyLeastActive BalancerPolicy = "least_active"
	// PolicyWeightedRoundRobin 平滑加权轮询
	PolicyWeightedRoundRobin BalancerPolicy = "weighted_round_robin"
)

// NewBalancer 按策略名创建负载均衡器
func NewBalancer(policy BalancerPolicy) Balancer {
	switch policy {
	case PolicyRandom:
		return RandomBalancer{}
	case PolicyLeastActive:
		return NewLeastActiveBalancer()
	case PolicyWeightedRoundRobin:
		return NewWeightedRoundRobinBalancer()
	default:
		return NewRoundRobinBalancer()
	}
}

// RoundRobinBalancer 原子轮询
type RoundRobinBalancer struct {
	n atomic.Uint64
}

// NewRoundRobinBalancer 创建轮询均衡器
func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{}
}

func (b *RoundRobinBalancer) Pick(candidates []*endpointNode) *endpointNode {
	if len(candidates) == 1 {
		return candidates[0]
	}
	i := b.n.Add(1) - 1
	return candidates[i%uint64(len(candidates))]
}

// RandomBalancer 均匀随机
type RandomBalancer struct{}

func (RandomBalancer) Pick(candidates []*endpointNode) *endpointNode {
	if len(candidates) == 1 {
		return candidates[0]
	}
	return candidates[rand.IntN(len(candidates))]
}

// LeastActiveBalancer 选择 inflight 最小的节点；并列时随机打散
type LeastActiveBalancer struct{}

// NewLeastActiveBalancer 创建最少活跃均衡器
func NewLeastActiveBalancer() LeastActiveBalancer {
	return LeastActiveBalancer{}
}

func (LeastActiveBalancer) Pick(candidates []*endpointNode) *endpointNode {
	if len(candidates) == 1 {
		return candidates[0]
	}
	best := candidates[0]
	bestLoad := best.Inflight()
	ties := 1
	for i := 1; i < len(candidates); i++ {
		c := candidates[i]
		load := c.Inflight()
		if load < bestLoad {
			best = c
			bestLoad = load
			ties = 1
		} else if load == bestLoad {
			ties++
			if rand.IntN(ties) == 0 {
				best = c
			}
		}
	}
	return best
}

// WeightedRoundRobinBalancer Nginx 式平滑加权轮询
type WeightedRoundRobinBalancer struct {
	state atomic.Pointer[wrrState]
}

type wrrState struct {
	addrs   []string
	weights []int
	current []int
	total   int
}

// NewWeightedRoundRobinBalancer 创建平滑加权轮询
func NewWeightedRoundRobinBalancer() *WeightedRoundRobinBalancer {
	return &WeightedRoundRobinBalancer{}
}

func (b *WeightedRoundRobinBalancer) Pick(candidates []*endpointNode) *endpointNode {
	if len(candidates) == 1 {
		return candidates[0]
	}

	for {
		st := b.state.Load()
		if st == nil || !wrrMatches(st, candidates) {
			st = buildWRRState(candidates)
			b.state.Store(st)
		}

		nextCurrent := make([]int, len(st.current))
		copy(nextCurrent, st.current)

		bestIdx := 0
		bestVal := -1 << 30
		for i := range st.weights {
			nextCurrent[i] += st.weights[i]
			if nextCurrent[i] > bestVal {
				bestVal = nextCurrent[i]
				bestIdx = i
			}
		}
		nextCurrent[bestIdx] -= st.total

		newSt := &wrrState{
			addrs:   st.addrs,
			weights: st.weights,
			current: nextCurrent,
			total:   st.total,
		}
		if b.state.CompareAndSwap(st, newSt) {
			want := st.addrs[bestIdx]
			for _, c := range candidates {
				if c.Addr() == want {
					return c
				}
			}
			return candidates[bestIdx%len(candidates)]
		}
	}
}

func wrrMatches(st *wrrState, candidates []*endpointNode) bool {
	if len(st.addrs) != len(candidates) {
		return false
	}
	for i, c := range candidates {
		if st.addrs[i] != c.Addr() || st.weights[i] != c.Weight() {
			return false
		}
	}
	return true
}

func buildWRRState(candidates []*endpointNode) *wrrState {
	st := &wrrState{
		addrs:   make([]string, len(candidates)),
		weights: make([]int, len(candidates)),
		current: make([]int, len(candidates)),
	}
	for i, c := range candidates {
		st.addrs[i] = c.Addr()
		st.weights[i] = c.Weight()
		st.total += st.weights[i]
	}
	return st
}
