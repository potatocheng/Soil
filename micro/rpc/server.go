package rpc

import (
	"context"
	"encoding/json"
	"net"
	"reflect"
)

type Server struct {
	network  string
	address  string
	services map[string]Service
}

func NewServer(network, address string) *Server {
	return &Server{
		network:  network,
		address:  address,
		services: make(map[string]Service, 20),
	}
}

// RegisterService 注册服务
func (s *Server) RegisterService(service Service) {
	s.services[service.Name()] = service
}

func (s *Server) Start() error {
	listener, err := net.Listen(s.network, s.address)
	if err != nil {
		return err
	}
	for {
		conn, er := listener.Accept()
		if er != nil {
			return er
		}
		go func() {
			if er = s.handleConn(conn); er != nil {
				_ = conn.Close()
			}
		}()
	}
}

func (s *Server) handleConn(conn net.Conn) error {
	// 读取客户端发送过来的信息
	data, err := Recv(conn)
	if err != nil {
		return err
	}

	req := &Request{}
	err = json.Unmarshal(data, req)
	if err != nil {
		return err
	}

	// 处理数据, 得到响应
	//respData, err := s.handleMsg(data)

	response, err := s.invoke(context.Background(), req)
	if err != nil {
		return err
	}
	// 封装数据
	resp := EncapsulatedData(response.Data)

	// 给请求方，返回响应
	_, err = conn.Write(resp)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) invoke(ctx context.Context, request *Request) (*Response, error) {
	service := s.services[request.ServiceName]
	method := reflect.ValueOf(service).MethodByName(request.MethodName)
	in := make([]reflect.Value, method.Type().NumIn())

	// TODO 将context传递到服务器
	in[0] = reflect.ValueOf(context.Background())
	in[1] = reflect.New(method.Type().In(1).Elem())
	err := json.Unmarshal(request.Args, in[1].Interface())
	if err != nil {
		return nil, err
	}
	results := method.Call(in)
	// results[0]是*Response, results[1]是error
	if results[1].Interface() != nil {
		return nil, results[1].Interface().(error)
	}
	responseData, err := json.Marshal(results[0].Interface())
	if err != nil {
		return nil, err
	}
	return &Response{Data: responseData}, nil
}
