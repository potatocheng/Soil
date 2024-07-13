package rpc

import "context"

type oneWayKey struct{}

func CtxWithOneWay(ctx context.Context) context.Context {
	return context.WithValue(ctx, oneWayKey{}, true)
}

func isOneWay(ctx context.Context) bool {
	val := ctx.Value(oneWayKey{})
	oneway, ok := val.(bool)
	return ok && oneway
}
