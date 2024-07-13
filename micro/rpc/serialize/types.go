package serialize

const (
	JsonSerializer  = 1
	ProtoSerializer = 2
)

type Serializer interface {
	Code() uint8
	Encode(value any) ([]byte, error)
	// Decode value接收结构体一级指针
	Decode(data []byte, value any) error
}
