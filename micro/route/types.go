package route

import (
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/resolver"
)

type Filter func(info balancer.PickInfo, address resolver.Address) bool

type GroupFilterBuilder struct{}

func (g *GroupFilterBuilder) Build() Filter {
	return func(info balancer.PickInfo, address resolver.Address) bool {
		clientTag := info.Ctx.Value("group").(string)
		serviceTag := address.Attributes.Value("group").(string)
		return serviceTag == clientTag
	}
}
