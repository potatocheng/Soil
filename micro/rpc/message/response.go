package message

import (
	"encoding/binary"
)

type Response struct {
	// 协议头部
	// 头部定长部分， 长度为15
	HeadLength uint32
	BodyLength uint32
	MessageID  uint32
	Version    uint8
	Compressor uint8
	Serializer uint8

	// 头部变长部分，错误信息
	ErrorInfo []byte

	// 响应数据
	Data []byte
}

func (r *Response) CalculateHeadLength() {
	r.HeadLength = uint32(15 + len(r.ErrorInfo))
}

func (r *Response) CalculateBodyLength() {
	r.BodyLength = uint32(len(r.Data))
}

// EncodeResponse 编码
func EncodeResponse(response *Response) []byte {
	bs := make([]byte, response.HeadLength+response.BodyLength)

	binary.BigEndian.PutUint32(bs[:4], response.HeadLength)
	binary.BigEndian.PutUint32(bs[4:8], response.BodyLength)
	binary.BigEndian.PutUint32(bs[8:12], response.MessageID)

	bs[12] = response.Version
	bs[13] = response.Compressor
	bs[14] = response.Serializer

	cur := bs[15:]
	copy(cur[:len(response.ErrorInfo)], response.ErrorInfo)
	cur = cur[len(response.ErrorInfo):]

	copy(cur, response.Data)

	return bs
}

// DecodeResponse 解码
func DecodeResponse(data []byte) *Response {
	response := new(Response)

	response.HeadLength = binary.BigEndian.Uint32(data[:4])
	response.BodyLength = binary.BigEndian.Uint32(data[4:8])
	response.MessageID = binary.BigEndian.Uint32(data[8:12])

	response.Version = data[12]
	response.Compressor = data[13]
	response.Serializer = data[14]
	// 协议头部的最后就是ErrorInfo，所以可以不用分隔符
	header := data[15:response.HeadLength]
	response.ErrorInfo = header

	if response.BodyLength > 0 {
		response.Data = data[response.HeadLength:]
	}

	return response
}
