package message

import (
	"bytes"
	"encoding/binary"
)

type Request struct {
	// 协议头部
	// 头部定长部分， 长度为15
	HeadLength uint32
	BodyLength uint32
	MessageID  uint32
	Version    uint8
	Compressor uint8
	Serializer uint8
	// 头部变长部分
	ServiceName string
	MethodName  string
	Meta        map[string]string

	// 协议体
	Data []byte
}

func (r *Request) CalculateHeadLength() {
	r.HeadLength = uint32(15 + len(r.ServiceName) + 1 + len(r.MethodName) + 1)
	for key, value := range r.Meta {
		r.HeadLength += uint32(len(key) + 1 + len(value) + 1)
	}
}

func (r *Request) CalculateBodyLength() {
	r.BodyLength = uint32(len(r.Data))
}

// EncodeRequest 编码
func EncodeRequest(request *Request) []byte {
	bs := make([]byte, request.HeadLength+request.BodyLength)
	binary.BigEndian.PutUint32(bs[0:4], request.HeadLength)
	binary.BigEndian.PutUint32(bs[4:8], request.BodyLength)
	binary.BigEndian.PutUint32(bs[8:12], request.MessageID)

	bs[12] = request.Version
	bs[13] = request.Compressor
	bs[14] = request.Serializer

	cur := bs[15:]
	// 编码协议头部的变长部分
	copy(cur[:len(request.ServiceName)], request.ServiceName)
	moveForward(&cur, len(request.ServiceName))

	copy(cur[:len(request.MethodName)], request.MethodName)
	moveForward(&cur, len(request.MethodName))

	// 编码头部中的元数据(键值对)，元数据键和值之间使用'\r'作为分割符,
	// 元数据键值对之间还是使用'\n'作为分割符
	for key, value := range request.Meta {
		copy(cur[:len(key)], key)
		cur = cur[len(key):]
		cur[0] = '\r'
		cur = cur[1:]
		copy(cur[:len(value)], value)
		moveForward(&cur, len(value))
	}

	// 编码 data部分
	if request.BodyLength > 0 {
		copy(cur, request.Data)
	}

	return bs
}

func moveForward(cur *[]byte, offset int) {
	*cur = (*cur)[offset:]
	//各个部分之间使用'\n'作为分隔符
	(*cur)[0] = '\n'
	*cur = (*cur)[1:]
}

// DecodeRequest 解码
func DecodeRequest(data []byte) *Request {
	request := new(Request)

	request.HeadLength = binary.BigEndian.Uint32(data[0:4])
	request.BodyLength = binary.BigEndian.Uint32(data[4:8])

	request.MessageID = binary.BigEndian.Uint32(data[8:12])

	request.Version = data[12]
	request.Compressor = data[13]
	request.Serializer = data[14]

	// 解码协议头部的变长部分
	header := data[15:request.HeadLength]
	index := bytes.IndexByte(header, '\n')
	request.ServiceName = string(header[:index])
	header = header[index+1:]

	index = bytes.IndexByte(header, '\n')
	request.MethodName = string(header[:index])
	header = header[index+1:]

	// 解码协议头部的元数据部分
	index = bytes.IndexByte(header, '\n')
	if -1 != index {
		meta := make(map[string]string)
		for -1 != index {
			pair := header[:index]
			pairSeparate := bytes.IndexByte(pair, '\r')
			key := pair[:pairSeparate]
			value := pair[pairSeparate+1:]
			meta[string(key)] = string(value)

			header = header[index+1:]
			index = bytes.IndexByte(header, '\n')
		}
		request.Meta = meta
	}
	if request.BodyLength > 0 {
		request.Data = data[request.HeadLength:]
	}

	return request
}
