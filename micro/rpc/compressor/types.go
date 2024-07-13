package compressor

const (
	Gzip = 1
)

type Compressor interface {
	Code() uint8
	Compress(p []byte) ([]byte, error)
	Decompress(p []byte) ([]byte, error)
}
