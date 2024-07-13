package protobuf

import (
	"Soil/micro/rpc/serialize"
	"errors"
	"google.golang.org/protobuf/proto"
)

type Serializer struct {
}

func (s *Serializer) Code() uint8 {
	return serialize.ProtoSerializer
}

func (s *Serializer) Encode(value any) ([]byte, error) {
	msg, ok := value.(proto.Message)
	if !ok {
		return nil, errors.New("micro: 输入参数类型不是proto.Message")
	}
	return proto.Marshal(msg)
}

func (s *Serializer) Decode(data []byte, value any) error {
	msg, ok := value.(proto.Message)
	if !ok {
		return errors.New("micro: 输入参数类型不是proto.Message")
	}
	return proto.Unmarshal(data, msg)
}
