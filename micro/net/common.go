package net

import (
	"errors"
	"io"
	"net"
)

var (
	errPoolClosed   = errors.New("client: connection pool closed")
	errConnClosed   = errors.New("client: connection closed")
	errNoConnection = errors.New("client: no available connection")
	// ErrNoEndpoint 表示没有可用的后端地址
	ErrNoEndpoint = errors.New("client: no available endpoint")
)

// writeFull 向 w 中完整写入 data
func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

// readFrame 从 r 中读取一个 Frame
func readFrame(r io.Reader, lim frameLimits) (*Frame, error) {
	return DecodeFrameWithLimits(func(buf []byte) error {
		_, err := io.ReadFull(r, buf)
		return err
	}, lim)
}

// isClosedConnErr 判断是否为连接已关闭类错误
func isClosedConnErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	return false
}
