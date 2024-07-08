package rpc

import (
	"Soil/micro/rpc/message"
	"context"
)

type Proxy interface {
	invoke(ctx context.Context, request *message.Request) (*message.Response, error)
}
