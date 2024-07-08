package rpc

import (
	"encoding/binary"
	"net"
)

const numOfLengthBytes = 8

func Recv(conn net.Conn) ([]byte, error) {
	// 解析传输数据的长度
	lenBs := make([]byte, numOfLengthBytes)
	_, err := conn.Read(lenBs)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint64(lenBs)
	// 读取传输数据
	data := make([]byte, length)
	_, err = conn.Read(data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func ReceiveResponseStream(conn net.Conn) ([]byte, error) {
	// response前4个字节是协议头长度，紧接着的4个字节是协议体长度
	length := make([]byte, numOfLengthBytes)
	_, err := conn.Read(length)
	if err != nil {
		return nil, err
	}
	headerLength := binary.BigEndian.Uint32(length[:4])
	bodyLength := binary.BigEndian.Uint32(length[4:])
	// 获得Response的总长度，将Response读取出来
	requestStream := make([]byte, headerLength+bodyLength)
	copy(requestStream[:4], length[:4])
	copy(requestStream[4:numOfLengthBytes], length[4:])
	_, err = conn.Read(requestStream[numOfLengthBytes:])
	if err != nil {
		return nil, err
	}

	return requestStream, nil
}

func ReceiveRequestStream(conn net.Conn) ([]byte, error) {
	// response前4个字节是协议头长度，紧接着的4个字节是协议体长度
	length := make([]byte, numOfLengthBytes)
	_, err := conn.Read(length)
	if err != nil {
		return nil, err
	}
	headerLength := binary.BigEndian.Uint32(length[:4])
	bodyLength := binary.BigEndian.Uint32(length[4:])
	// 获得Response的总长度，将Response读取出来
	requestStream := make([]byte, headerLength+bodyLength)
	copy(requestStream[:4], length[:4])
	copy(requestStream[4:numOfLengthBytes], length[4:])
	_, err = conn.Read(requestStream[numOfLengthBytes:])
	if err != nil {
		return nil, err
	}

	return requestStream, nil
}

func EncapsulatedData(data []byte) []byte {
	dataLength := len(data)
	res := make([]byte, numOfLengthBytes+dataLength)
	// 将数据长度放在返回结果前面
	binary.BigEndian.PutUint64(res[:numOfLengthBytes], uint64(dataLength))
	// 将数据放到返回结果中
	copy(res[numOfLengthBytes:], data)

	return res
}
