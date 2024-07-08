package rpc

import (
	"Soil/micro/rpc/message"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
)

type Server struct {
	network  string
	address  string
	services map[string]reflectStub
}

func NewServer(network, address string) *Server {
	return &Server{
		network:  network,
		address:  address,
		services: make(map[string]reflectStub, 20),
	}
}

// RegisterService 注册服务
func (s *Server) RegisterService(service Service) {
	s.services[service.Name()] = reflectStub{
		s:     service,
		value: reflect.ValueOf(service),
	}
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
	for {
		// 读取客户端发送过来请求
		data, err := ReceiveRequestStream(conn)
		if err != nil {
			return err
		}

		// 解析请求
		req := message.DecodeRequest(data)

		response, businessExecErr := s.invoke(context.Background(), req)
		if businessExecErr != nil {
			response.ErrorInfo = []byte(businessExecErr.Error())
		}

		if response != nil {
			response.CalculateBodyLength()
			response.CalculateHeadLength()
		}

		// 封装数据
		respStream := message.EncodeResponse(response)

		// 给请求方，返回响应
		_, err = conn.Write(respStream)
		if err != nil {
			return err
		}
	}
}

func (s *Server) invoke(ctx context.Context, request *message.Request) (*message.Response, error) {
	serviceStub, ok := s.services[request.ServiceName]
	resp := &message.Response{
		MessageID:  request.MessageID,
		Version:    request.Version,
		Compressor: request.Compressor,
		Serializer: request.Serializer,
	}
	if !ok {
		return resp, fmt.Errorf("micro: 服务[%s]不存在", request.ServiceName)
	}

	responseData, err := serviceStub.invoke(ctx, request)
	resp.Data = responseData
	if err != nil {
		return resp, err
	}
	return resp, nil
}

type reflectStub struct {
	s     Service
	value reflect.Value
}

func (r *reflectStub) invoke(ctx context.Context, request *message.Request) ([]byte, error) {
	method := r.value.MethodByName(request.MethodName)
	in := make([]reflect.Value, method.Type().NumIn())

	// TODO 将context传递到服务器
	in[0] = reflect.ValueOf(context.Background())
	in[1] = reflect.New(method.Type().In(1).Elem())
	err := json.Unmarshal(request.Data, in[1].Interface())
	if err != nil {
		return nil, err
	}
	results := method.Call(in)
	// results[0]是*Response, results[1]是error
	if results[1].Interface() != nil {
		err = results[1].Interface().(error)
	}

	var responseData []byte
	if results[0].IsNil() {
		return nil, err
	} else {
		var jsonErr error
		responseData, jsonErr = json.Marshal(results[0].Interface())
		if jsonErr != nil {
			return nil, jsonErr
		}
	}
	return responseData, err
}
