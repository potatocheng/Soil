package json

import (
	"Soil/micro/rpc/serialize"
	"encoding/json"
)

type Serializer struct {
}

func (s *Serializer) Code() uint8 {
	return serialize.JsonSerializer
}

func (s *Serializer) Encode(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (s *Serializer) Decode(data []byte, value any) error {
	return json.Unmarshal(data, value)
}
