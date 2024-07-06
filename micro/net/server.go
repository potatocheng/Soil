package net

import "net"

type Server struct {
	network string
	address string
}

func NewServer(network, address string) *Server {
	return &Server{
		network: network,
		address: address,
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
	// 读取客户端发送过来的信息
	data, err := Recv(conn)
	if err != nil {
		return err
	}

	// 处理数据, 得到响应
	respData := handleMsg(data)

	// 封装数据
	resp := EncapsulatedData(respData)

	// 给请求方，返回响应
	_, err = conn.Write(resp)
	if err != nil {
		return err
	}

	return nil
}

func handleMsg(data []byte) []byte {
	return data
}
