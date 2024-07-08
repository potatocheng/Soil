package rpc

import (
	"Soil/micro/pool"
	"Soil/micro/rpc/message"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"reflect"
	"time"
)

func InitClientProxy(addr string, service Service) error {
	if service == nil {
		return errors.New("rpc: service is nil")
	}

	//在这里需要创建一个proxy
	client, err := NewClient(addr)
	if err != nil {
		return err
	}
	return setFuncFiled(service, client)
}

func setFuncFiled(service Service, proxy Proxy) error {
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
				reqData, err := json.Marshal(in[1].Interface())
				if err != nil {
					return []reflect.Value{response, reflect.ValueOf(err)}
				}
				req := &message.Request{
					ServiceName: service.Name(),
					MethodName:  fieldType.Name,
					Data:        reqData,
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
					err = json.Unmarshal(resp.Data, response.Interface())
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
	pool *pool.Pool
}

func NewClient(addr string) (*Client, error) {
	factory := func() (net.Conn, error) {
		return net.DialTimeout("tcp", addr, time.Second*3)
	}
	p, err := pool.NewPool(1, 10, 20, time.Minute, factory, nil)
	if err != nil {
		return nil, err
	}

	return &Client{
		pool: p,
	}, nil
}

// invoke 发起请求
func (c *Client) invoke(ctx context.Context, request *message.Request) (*message.Response, error) {
	reqBs := message.EncodeRequest(request)
	// 将请求发送到服务器
	respStream, err := c.SendAndReceive(reqBs)
	if err != nil {
		return nil, err
	}
	return message.DecodeResponse(respStream), nil
}

func (c *Client) SendAndReceive(data []byte) ([]byte, error) {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	conn, err := c.pool.Get(ctx)
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

	return ReceiveResponseStream(conn)
}
