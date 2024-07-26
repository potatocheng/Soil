package micro

import (
	"Soil/micro/registry"
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"time"
)

type Client struct {
	r       registry.Registry
	timeout time.Duration
}

type ClientOption func(*Client)

func NewClient(opts ...ClientOption) (*Client, error) {
	res := &Client{}

	for _, opt := range opts {
		opt(res)
	}

	return res, nil
}

func ClientWithRegistry(r registry.Registry, timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.r = r
		c.timeout = timeout
	}
}

func (c *Client) Dial(ctx context.Context, serviceName string, dialOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	if c.r != nil {
		builder, err := NewResolverBuilder(c.r, c.timeout)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.WithResolvers(builder))
	}

	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	if len(dialOpts) > 0 {
		opts = append(opts, dialOpts...)
	}

	return grpc.NewClient(fmt.Sprintf("registry:///%s", serviceName), opts...)
}
