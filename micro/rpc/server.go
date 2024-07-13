package rpc

import (
	"Soil/micro/rpc/compressor"
	"Soil/micro/rpc/compressor/gzipCompressor"
	"Soil/micro/rpc/message"
	"Soil/micro/rpc/serialize"
	"Soil/micro/rpc/serialize/json"
	"Soil/micro/rpc/serialize/protobuf"
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"time"
)

type Server struct {
	network     string
	address     string
	services    map[string]reflectStub
	serializers map[uint8]serialize.Serializer
	compressors map[uint8]compressor.Compressor
}

func NewServer(network, address string) *Server {
	res := &Server{
		network:     network,
		address:     address,
		services:    make(map[string]reflectStub, 10),
		serializers: make(map[uint8]serialize.Serializer, 2),
		compressors: make(map[uint8]compressor.Compressor, 2),
	}
	res.RegisterSerializer(&json.Serializer{})
	res.RegisterSerializer(&protobuf.Serializer{})

	res.RegisterCompressor(&gzipCompressor.Compressor{})

	return res
}

// RegisterService 注册服务
func (s *Server) RegisterService(service Service) {
	s.services[service.Name()] = reflectStub{
		s:           service,
		value:       reflect.ValueOf(service),
		serializers: s.serializers,
		compressors: s.compressors,
	}
}

func (s *Server) RegisterSerializer(serializer serialize.Serializer) {
	s.serializers[serializer.Code()] = serializer
}

func (s *Server) RegisterCompressor(compressor compressor.Compressor) {
	s.compressors[compressor.Code()] = compressor
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

		ctx := context.Background()
		// 处理链路超时
		var cancel context.CancelFunc
		if deadlineStr, ok := req.Meta["deadline"]; ok {
			deadline, er := strconv.ParseInt(deadlineStr, 10, 64)
			if er == nil {
				ctx, cancel = context.WithDeadline(ctx, time.UnixMilli(deadline))
			}
		}

		// 处理one-way
		f, ok := req.Meta["one-way"]
		if ok && f == "true" {
			ctx = CtxWithOneWay(ctx)
		}
		response, businessExecErr := s.invoke(ctx, req)
		if cancel != nil {
			cancel()
		}

		if ok && f == "true" {
			continue
		}

		if businessExecErr != nil {
			response.ErrorInfo = []byte(businessExecErr.Error())
		}

		response.CalculateBodyLength()
		response.CalculateHeadLength()

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

	// one-way并不关心业务执行结果
	if isOneWay(ctx) {
		go func() {
			_, _ = serviceStub.invoke(ctx, request)
		}()
		return nil, errors.New("micro: 微服务服务端 oneway 请求")
	}

	responseData, err := serviceStub.invoke(ctx, request)
	resp.Data = responseData
	if err != nil {
		return resp, err
	}
	return resp, nil
}

type reflectStub struct {
	s           Service
	value       reflect.Value
	serializers map[uint8]serialize.Serializer
	compressors map[uint8]compressor.Compressor
}

// invoke 处理业务
func (r *reflectStub) invoke(ctx context.Context, request *message.Request) ([]byte, error) {
	method := r.value.MethodByName(request.MethodName)
	in := make([]reflect.Value, method.Type().NumIn())

	// TODO 将context传递到服务器
	in[0] = reflect.ValueOf(ctx)
	in[1] = reflect.New(method.Type().In(1).Elem())
	serializer := r.serializers[request.Serializer]
	compress, compressOk := r.compressors[request.Compressor]

	reqData := request.Data
	var err error

	if compressOk {
		// 如果设置了压缩算法
		reqData, err = compress.Decompress(reqData)
	}
	if err != nil {
		return nil, err
	}
	err = serializer.Decode(reqData, in[1].Interface())
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
		responseData, jsonErr = serializer.Encode(results[0].Interface())
		if jsonErr != nil {
			return nil, jsonErr
		}
	}
	return responseData, err
}
