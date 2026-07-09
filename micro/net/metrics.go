package net

import "time"

// Metrics 是指标接入点，默认使用 no-op
type Metrics interface {
	// ConnectionOpened 连接建立
	ConnectionOpened(role string)
	// ConnectionClosed 连接关闭
	ConnectionClosed(role string)
	// RequestHandled 请求处理完成
	RequestHandled(duration time.Duration, err error)
	// RequestSent 请求发送完成
	RequestSent(duration time.Duration, err error)
}

type noopMetrics struct{}

func (n noopMetrics) ConnectionOpened(_ string)               {}
func (n noopMetrics) ConnectionClosed(_ string)               {}
func (n noopMetrics) RequestHandled(_ time.Duration, _ error) {}
func (n noopMetrics) RequestSent(_ time.Duration, _ error)    {}

var defaultMetrics Metrics = noopMetrics{}
