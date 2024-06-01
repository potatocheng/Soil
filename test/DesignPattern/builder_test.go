package DesignPattern

import (
	"crypto/tls"
	"fmt"
	"testing"
	"time"
)

type Server struct {
	Addr     string
	Port     int
	Protocol string
	Timeout  time.Duration
	MaxConns int
	TLS      *tls.Config
}

// 由于没有构造函数和参数默认值所以构造对象时，我们需要有多种不同的创建不同配置 Server 的函数签名
func NewDefaultServer(addr string, port int) (*Server, error) {
	return &Server{addr, port, "tcp", 30 * time.Second, 100, nil}, nil
}

// 由于Go又不支持函数重载，还得用不同的函数名来对应不同的配置选项
func NewTLSServer(addr string, port int, tlsConfig *tls.Config) (*Server, error) {
	return &Server{addr, port, "tcp", 30 * time.Second, 100, tlsConfig}, nil
}

//....

// builder模式
type ServerBuilder struct {
	Server //匿名嵌入(组合)，可以直接访问嵌入结构体的字段和方法
}

func (sb *ServerBuilder) Create(addr string, port int) *ServerBuilder {
	sb.Server.Addr = addr
	sb.Server.Port = port

	return sb
}

func (sb *ServerBuilder) WithProtocol(protocol string) *ServerBuilder {
	sb.Server.Protocol = protocol
	return sb
}

func (sb *ServerBuilder) WithTimeout(timeout time.Duration) *ServerBuilder {
	sb.Server.Timeout = timeout
	return sb
}

func (sb *ServerBuilder) WithTLS(tlsConfig *tls.Config) *ServerBuilder {
	sb.Server.TLS = tlsConfig
	return sb
}

func (sb *ServerBuilder) WithMaxConns(max int) *ServerBuilder {
	sb.Server.MaxConns = max
	return sb
}

func (sb *ServerBuilder) Build() Server {
	return sb.Server
}

func Test_Builder(t *testing.T) {
	//初始化
	sb := ServerBuilder{}
	ser := sb.Create("0.0.0.0", 8080).
		WithProtocol("tcp").WithMaxConns(1024).Build()

	fmt.Println(ser)
}

// Function Options
type Option func(*Server)

func Protocol(protocol string) Option {
	return func(s *Server) {
		s.Protocol = protocol
	}
}

func Timeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.Timeout = timeout
	}
}

func MaxConns(max int) Option {
	return func(s *Server) {
		s.MaxConns = max
	}
}

func TLS(tlsConfig *tls.Config) Option {
	return func(s *Server) {
		s.TLS = tlsConfig
	}
}

func NewServer(addr string, port int, options ...func(*Server)) (*Server, error) {
	srv := Server{
		Addr:     addr,
		Port:     port,
		Protocol: "tcp",
		Timeout:  30 * time.Second,
		MaxConns: 1024,
		TLS:      nil,
	}

	for _, option := range options {
		option(&srv)
	}

	return &srv, nil
}

func Test_Option(t *testing.T) {
	s1, _ := NewServer("localhost", 1024)
	fmt.Println(s1)
	s2, _ := NewServer("localhost", 1024, Protocol("upd"))
	fmt.Println(s2)
}
