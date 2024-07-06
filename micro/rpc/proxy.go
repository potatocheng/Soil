package rpc

import "context"

type Proxy interface {
	invoke(ctx context.Context, request *Request) (*Response, error)
}
