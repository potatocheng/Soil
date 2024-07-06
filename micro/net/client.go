package net

import (
	"net"
	"time"
)

type Client struct {
	network string
	address string
	conn    net.Conn
}

func NewClient(network, address string) *Client {
	return &Client{
		network: network,
		address: address,
	}
}

func (c *Client) Send(data string) error {
	var err error
	c.conn, err = net.DialTimeout(c.network, c.address, time.Second*3)
	if err != nil {
		return err
	}
	// 封装数据
	req := EncapsulatedData([]byte(data))
	// 发送数据
	_, err = c.conn.Write(req)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) Receive() (string, error) {
	resp, err := Recv(c.conn)
	return string(resp), err
}

func (c *Client) Communicate(data string) (string, error) {
	err := c.Send(data)
	if err != nil {
		return "", err
	}
	return c.Receive()
}
