package net

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
)

// MessageType 定义消息类型
type MessageType uint8

const (
	MessageTypeRequest MessageType = iota + 1
	MessageTypeResponse
	MessageTypeHeartbeat
	MessageTypePing
	MessageTypePong
)

// 协议常量
const (
	frameMagic     uint16 = 0x4D4E // 'M' 'N'
	frameVersion   uint8  = 1
	frameHeaderLen        = 22 // 2+1+1+1+1+4+4+8

	defaultMaxBodySize   = 16 * 1024 * 1024 // 16 MB
	defaultMaxHeaderSize = 64 * 1024        // 64 KB
)

// Flags 定义
const (
	FlagOneWay uint8 = 1 << iota
	FlagError
)

// 协议层哨兵错误
var (
	ErrInvalidMagic   = errors.New("invalid frame magic")
	ErrLargeMessage   = errors.New("message too large")
	ErrLargeHeader    = errors.New("header too large")
	ErrLargeBody      = errors.New("body too large")
	ErrUnknownVersion = errors.New("unsupported protocol version")
	ErrRemote         = errors.New("remote error")
	ErrRateLimited    = errors.New("rate limited")
	ErrHandlerPanic   = errors.New("handler panic")
)

// frameLimits 控制单帧大小上限
type frameLimits struct {
	maxHeader uint32
	maxBody   uint32
}

func defaultLimits() frameLimits {
	return frameLimits{
		maxHeader: defaultMaxHeaderSize,
		maxBody:   defaultMaxBodySize,
	}
}

// Frame 是 micro/net 协议帧
type Frame struct {
	Magic     uint16
	Version   uint8
	MsgType   MessageType
	Flags     uint8
	RequestID uint64
	Header    []byte
	Body      []byte
}

// NewFrame 创建一个默认 Frame
func NewFrame(msgType MessageType, requestID uint64, header, body []byte) *Frame {
	return &Frame{
		Magic:     frameMagic,
		Version:   frameVersion,
		MsgType:   msgType,
		RequestID: requestID,
		Header:    header,
		Body:      body,
	}
}

// NewErrorFrame 创建错误响应帧
func NewErrorFrame(requestID uint64, err error) *Frame {
	msg := "unknown error"
	if err != nil {
		msg = err.Error()
	}
	f := NewFrame(MessageTypeResponse, requestID, nil, []byte(msg))
	f.Flags |= FlagError
	return f
}

// Encode 将 Frame 编码为字节流（兼容测试与调试；热路径优先用 writeFrame）
func (f *Frame) Encode() ([]byte, error) {
	return f.EncodeWithLimits(defaultLimits())
}

// EncodeWithLimits 按给定上限编码
func (f *Frame) EncodeWithLimits(lim frameLimits) ([]byte, error) {
	if err := f.validate(lim); err != nil {
		return nil, err
	}

	headerLen := len(f.Header)
	bodyLen := len(f.Body)
	totalLen := frameHeaderLen + headerLen + bodyLen

	buf := make([]byte, totalLen)
	writeHeaderFields(buf, f, uint32(headerLen), uint32(bodyLen))
	copy(buf[frameHeaderLen:], f.Header)
	copy(buf[frameHeaderLen+headerLen:], f.Body)
	return buf, nil
}

func (f *Frame) validate(lim frameLimits) error {
	if f.Magic != 0 && f.Magic != frameMagic {
		return fmt.Errorf("%w: 0x%04X", ErrInvalidMagic, f.Magic)
	}
	if f.Version != 0 && f.Version != frameVersion {
		return fmt.Errorf("%w: %d", ErrUnknownVersion, f.Version)
	}
	if uint32(len(f.Header)) > lim.maxHeader {
		return fmt.Errorf("%w: %d > %d", ErrLargeHeader, len(f.Header), lim.maxHeader)
	}
	if uint32(len(f.Body)) > lim.maxBody {
		return fmt.Errorf("%w: %d > %d", ErrLargeBody, len(f.Body), lim.maxBody)
	}
	return nil
}

func writeHeaderFields(buf []byte, f *Frame, headerLen, bodyLen uint32) {
	binary.BigEndian.PutUint16(buf[0:], f.Magic)
	buf[2] = f.Version
	buf[3] = uint8(f.MsgType)
	buf[4] = f.Flags
	buf[5] = 0 // reserved
	binary.BigEndian.PutUint32(buf[6:], headerLen)
	binary.BigEndian.PutUint32(buf[10:], bodyLen)
	binary.BigEndian.PutUint64(buf[14:], f.RequestID)
}

// headerScratch 复用 22 字节帧头缓冲
var headerScratchPool = sync.Pool{
	New: func() any {
		b := make([]byte, frameHeaderLen)
		return &b
	},
}

// writeFrame 将帧写入 w：固定头走栈/池，body 零拷贝写出，避免整帧二次分配
func writeFrame(w io.Writer, f *Frame, lim frameLimits) error {
	if err := f.validate(lim); err != nil {
		return err
	}

	hp := headerScratchPool.Get().(*[]byte)
	hdr := (*hp)[:frameHeaderLen]
	writeHeaderFields(hdr, f, uint32(len(f.Header)), uint32(len(f.Body)))

	if err := writeFull(w, hdr); err != nil {
		headerScratchPool.Put(hp)
		return err
	}
	headerScratchPool.Put(hp)

	if len(f.Header) > 0 {
		if err := writeFull(w, f.Header); err != nil {
			return err
		}
	}
	if len(f.Body) > 0 {
		if err := writeFull(w, f.Body); err != nil {
			return err
		}
	}
	return nil
}

// DecodeFrame 从 readFull 回调解码一个 Frame
func DecodeFrame(readFull func([]byte) error) (*Frame, error) {
	return DecodeFrameWithLimits(readFull, defaultLimits())
}

// DecodeFrameWithLimits 按给定上限解码
func DecodeFrameWithLimits(readFullFn func([]byte) error, lim frameLimits) (*Frame, error) {
	hp := headerScratchPool.Get().(*[]byte)
	header := (*hp)[:frameHeaderLen]
	err := readFullFn(header)
	if err != nil {
		headerScratchPool.Put(hp)
		return nil, err
	}

	f := &Frame{}
	f.Magic = binary.BigEndian.Uint16(header[0:])
	f.Version = header[2]
	f.MsgType = MessageType(header[3])
	f.Flags = header[4]
	// header[5] reserved
	headerLen := binary.BigEndian.Uint32(header[6:])
	bodyLen := binary.BigEndian.Uint32(header[10:])
	f.RequestID = binary.BigEndian.Uint64(header[14:])
	headerScratchPool.Put(hp)

	if f.Magic != frameMagic {
		return nil, fmt.Errorf("%w: 0x%04X", ErrInvalidMagic, f.Magic)
	}
	if f.Version != frameVersion {
		return nil, fmt.Errorf("%w: %d", ErrUnknownVersion, f.Version)
	}
	if headerLen > lim.maxHeader {
		return nil, fmt.Errorf("%w: %d", ErrLargeHeader, headerLen)
	}
	if bodyLen > lim.maxBody {
		return nil, fmt.Errorf("%w: %d", ErrLargeBody, bodyLen)
	}

	if headerLen > 0 {
		f.Header = make([]byte, headerLen)
		if err := readFullFn(f.Header); err != nil {
			return nil, err
		}
	}
	if bodyLen > 0 {
		f.Body = make([]byte, bodyLen)
		if err := readFullFn(f.Body); err != nil {
			return nil, err
		}
	}

	return f, nil
}

// IsOneWay 返回是否为单向消息
func (f *Frame) IsOneWay() bool {
	return f.Flags&FlagOneWay != 0
}

// IsError 返回是否为错误响应
func (f *Frame) IsError() bool {
	return f.Flags&FlagError != 0
}

// IsHeartbeat 返回是否为心跳消息
func (f *Frame) IsHeartbeat() bool {
	return f.MsgType == MessageTypeHeartbeat || f.MsgType == MessageTypePing || f.MsgType == MessageTypePong
}

// RemoteError 表示对端返回的业务/协议错误
type RemoteError struct {
	RequestID uint64
	Message   string
}

func (e *RemoteError) Error() string {
	if e.Message == "" {
		return ErrRemote.Error()
	}
	return e.Message
}

func (e *RemoteError) Unwrap() error { return ErrRemote }

func remoteErrorFromFrame(f *Frame) error {
	msg := string(f.Body)
	switch msg {
	case ErrRateLimited.Error():
		return fmt.Errorf("%w: %w", ErrRemote, ErrRateLimited)
	case ErrHandlerPanic.Error():
		return fmt.Errorf("%w: %w", ErrRemote, ErrHandlerPanic)
	default:
		return &RemoteError{RequestID: f.RequestID, Message: msg}
	}
}
