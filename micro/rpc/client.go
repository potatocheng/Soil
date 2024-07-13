package rpc

import (
	"Soil/micro/pool"
	"Soil/micro/rpc/compressor"
	"Soil/micro/rpc/message"
	"Soil/micro/rpc/serialize"
	"Soil/micro/rpc/serialize/json"
	"context"
	"errors"
	"log"
	"net"
	"reflect"
	"strconv"
	"time"
)

func (c *Client) InitClientProxy(service Service) error {
	if service == nil {
		return errors.New("rpc: service is nil")
	}

	return setFuncFiled(service, c, c.serializer, c.compressor)
}

func setFuncFiled(service Service, proxy Proxy, serializer serialize.Serializer, compressor compressor.Compressor) error {
	serviceReflectValue := reflect.ValueOf(service)
	serviceReflectType := serviceReflectValue.Type()
	if serviceReflectType.Kind() != reflect.Pointer ||
		serviceReflectType.Elem().Kind() != reflect.Struct {
		return errors.New("rpc: service must be a pointer to a struct")
	}

	serviceReflectType = serviceReflectType.Elem()
	serviceReflectValue = serviceReflectValue.Elem()

	fieldNum := serviceReflectValue.NumField()
	for i := 0; i < fieldNum; i++ {
		fieldType := serviceReflectType.Field(i)
		fieldValue := serviceReflectValue.Field(i)

		if fieldValue.CanSet() {
			fn := func(in []reflect.Value) []reflect.Value {
				response := reflect.New(fieldType.Type.Out(0).Elem())
				// 构建请求
				// in[0]表示context
				ctx := in[0].Interface().(context.Context)
				// in[1]表示参数, 将in[1]序列化
				reqData, err := serializer.Encode(in[1].Interface())
				if err != nil {
					return []reflect.Value{response, reflect.ValueOf(err)}
				}
				// 设置了压缩算法, 压缩请求数据
				if compressor != nil {
					reqData, err = compressor.Compress(reqData)
					if err != nil {
						return []reflect.Value{response, reflect.ValueOf(err)}
					}
				}

				meta := make(map[string]string, 2)
				if deadline, ok := ctx.Deadline(); ok {
					meta["deadline"] = strconv.FormatInt(deadline.UnixMilli(), 10)
				}

				if isOneWay(ctx) {
					meta["one-way"] = "true"
				}

				req := &message.Request{
					ServiceName: service.Name(),
					MethodName:  fieldType.Name,
					Serializer:  serializer.Code(),
					Meta:        meta,
					Data:        reqData,
				}

				if compressor != nil {
					req.Compressor = compressor.Code()
				}

				req.CalculateHeadLength()
				req.CalculateBodyLength()

				// 发起调用
				resp, err := proxy.invoke(ctx, req)
				if err != nil {
					return []reflect.Value{response, reflect.ValueOf(err)}
				}

				var businessExecErrVal reflect.Value
				if len(resp.ErrorInfo) > 0 {
					// 业务处理出错
					businessExecErrVal = reflect.ValueOf(errors.New(string(resp.ErrorInfo)))
				} else {
					businessExecErrVal = reflect.Zero(reflect.TypeOf(new(error)).Elem())
				}

				if len(resp.Data) > 0 {
					// 解析出response
					err = serializer.Decode(resp.Data, response.Interface())
					if err != nil {
						return []reflect.Value{response, reflect.ValueOf(err)}
					}
				}

				return []reflect.Value{response, businessExecErrVal}
			}
			fnVal := reflect.MakeFunc(fieldType.Type, fn)
			fieldValue.Set(fnVal)
		}
	}

	return nil
}

type Client struct {
	pool       *pool.Pool
	serializer serialize.Serializer
	compressor compressor.Compressor
}

type ClientOption func(c *Client)

func ClientWithSerializer(serializer serialize.Serializer) ClientOption {
	return func(c *Client) {
		c.serializer = serializer
	}
}

func ClientWithCompressor(compressor compressor.Compressor) ClientOption {
	return func(c *Client) {
		c.compressor = compressor
	}
}

// NewClient 默认的序列化方法是json, 压缩方法默认不开启
func NewClient(addr string, opts ...ClientOption) (*Client, error) {
	factory := func() (net.Conn, error) {
		return net.DialTimeout("tcp", addr, time.Second*3)
	}
	p, err := pool.NewPool(1, 10, 20, time.Minute, factory, nil)
	if err != nil {
		return nil, err
	}

	res := &Client{
		pool:       p,
		serializer: &json.Serializer{},
	}
	for _, opt := range opts {
		opt(res)
	}

	return res, nil
}

// invoke 发起请求
func (c *Client) invoke(ctx context.Context, request *message.Request) (*message.Response, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	ch := make(chan struct{}, 1)
	defer close(ch)

	var (
		response *message.Response
		err      error
	)
	// 这样开goroutine的方式会导致超时发生后，并不会终中断正在执行的调用
	// 只是客户端丢掉了调用的响应
	go func() {
		response, err = c.doInvoke(ctx, request)
		ch <- struct{}{}
	}()

	select {
	case <-ch:
		// 服务端返回结果
		return response, err
	case <-ctx.Done():
		// 链路超时
		return nil, ctx.Err()
	}
}

func (c *Client) doInvoke(ctx context.Context, request *message.Request) (*message.Response, error) {
	reqBs := message.EncodeRequest(request)
	// 将请求发送到服务器
	respStream, err := c.sendAndReceive(ctx, reqBs)
	if err != nil {
		return nil, err
	}
	return message.DecodeResponse(respStream), nil
}

func (c *Client) sendAndReceive(ctx context.Context, data []byte) ([]byte, error) {
	var err error
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
	conn, err := c.pool.Get(ctxTimeout)
	cancel()
	if err != nil {
		return nil, err
	}
	defer func() {
		err = c.pool.Put(conn)
		if err != nil {
			log.Println("rpc: 将连接放入连接池失败")
		}
	}()

	// 发送数据
	_, err = conn.Write(data)
	if err != nil {
		return nil, err
	}

	// 如果是one-way调用就不必等待服务器的返回值，直接在这里就可以返回
	if isOneWay(ctx) {
		return nil, errors.New("mirco: one-way调用，不必处理结果")
	}

	return ReceiveResponseStream(conn)
}
