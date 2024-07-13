package gzipCompressor

import (
	"Soil/micro/rpc/compressor"
	"bytes"
	"compress/gzip"
	"io"
)

var _ compressor.Compressor = &Compressor{}

type Compressor struct {
}

func (c *Compressor) Code() uint8 {
	return compressor.Gzip
}

// Compress 压缩
func (c *Compressor) Compress(p []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(p)
	if err != nil {
		return nil, err
	}
	if err = zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decompress 解压
func (c *Compressor) Decompress(p []byte) ([]byte, error) {
	buf := bytes.NewBuffer(p)
	zr, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}

	defer func() {
		err = zr.Close()
	}()

	var result bytes.Buffer
	_, err = io.Copy(&result, zr)

	return result.Bytes(), err
}
