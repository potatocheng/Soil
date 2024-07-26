package micro

import (
	"Soil/micro/registry"
	"context"
	"google.golang.org/grpc"
	"net"
	"time"
)

type Server struct {
	name            string
	registry        registry.Registry
	registerTimeout time.Duration
	*grpc.Server
	listener net.Listener
	weight   uint32
	group    string
}

type ServerOption func(s *Server)

func ServerWithRegistry(reg registry.Registry) ServerOption {
	return func(s *Server) {
		s.registry = reg
	}
}

func ServerWithWeight(weight uint32) ServerOption {
	return func(s *Server) {
		s.weight = weight
	}
}

func ServerWithGroup(group string) ServerOption {
	return func(s *Server) {
		s.group = group
	}
}

func NewServer(serviceName string, registerTimeout time.Duration, opts ...ServerOption) (*Server, error) {
	res := &Server{
		name:            serviceName,
		Server:          grpc.NewServer(),
		registerTimeout: registerTimeout,
	}

	for _, opt := range opts {
		opt(res)
	}

	return res, nil
}

func (s *Server) Start(addr string) error {
	var err error
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	if s.registry != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.registerTimeout)
		defer cancel()
		err = s.registry.Registry(ctx, registry.ServiceInstance{
			Name:    s.name,
			Address: s.listener.Addr().String(),
			Weight:  s.weight,
			Group:   s.group,
		})
		if err != nil {
			return err
		}
	}

	err = s.Serve(s.listener)
	return err
}

func (s *Server) Close() error {
	if s.registry != nil {
		err := s.registry.Close()
		if err != nil {
			return err
		}
	}
	s.GracefulStop()
	return nil
}
