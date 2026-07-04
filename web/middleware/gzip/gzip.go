package gzip

import (
	"Soil/web"
	"bytes"
	"compress/gzip"
	"strings"
)

// MiddlewareBuilder 用于构建 Gzip 压缩中间件。
type MiddlewareBuilder struct {
	// MinSize 是触发压缩的最小响应字节数，默认 1024。
	MinSize int
	// Level 是 gzip 压缩级别，默认 gzip.DefaultCompression。
	Level int
}

// Create 创建 MiddlewareBuilder，默认 MinSize=1024，Level=gzip.DefaultCompression。
func Create() *MiddlewareBuilder {
	return &MiddlewareBuilder{
		MinSize: 1024,
		Level:   gzip.DefaultCompression,
	}
}

// WithMinSize 链式设置最小压缩阈值。
func (mb *MiddlewareBuilder) WithMinSize(size int) *MiddlewareBuilder {
	mb.MinSize = size
	return mb
}

// WithLevel 链式设置压缩级别。
func (mb *MiddlewareBuilder) WithLevel(level int) *MiddlewareBuilder {
	mb.Level = level
	return mb
}

// Build 构建 Gzip 压缩中间件。
//
// 适配 Soil 架构：Soil 的响应机制是 handler 设置 ctx.RespStatusCode 与 ctx.RespData，
// 由 server.go 中的 flashResp 统一写出。ServeHTTP 中 flashResp 由最外层包装中间件 m
// 在 next(ctx) 返回之后调用，因此用户中间件在 next(ctx) 之后修改 ctx.RespData，
// flashResp 会写出修改（压缩）后的数据。故本中间件无需包装 ctx.Resp。
func (mb *MiddlewareBuilder) Build() web.Middleware {
	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			// 检查客户端是否接受 gzip 编码
			if !strings.Contains(ctx.Req.Header.Get("Accept-Encoding"), "gzip") {
				next(ctx)
				return
			}

			// 先执行后续 handler，生成 ctx.RespData 与 RespHeaders
			next(ctx)

			// 响应体过小不压缩
			if len(ctx.RespData) < mb.MinSize {
				return
			}

			// 仅压缩文本类内容，已压缩的二进制内容不重复压缩
			contentType := ctx.RespHeaders.Get("Content-Type")
			if !shouldCompress(contentType) {
				return
			}

			// 使用 gzip 压缩响应体
			var buf bytes.Buffer
			w, err := gzip.NewWriterLevel(&buf, mb.Level)
			if err != nil {
				// 压缩级别非法时不压缩，直接返回原始响应
				return
			}
			_, _ = w.Write(ctx.RespData)
			_ = w.Close()

			ctx.RespData = buf.Bytes()
			ctx.SetHeader("Content-Encoding", "gzip")
			ctx.SetHeader("Vary", "Accept-Encoding")
			// 压缩后 Content-Length 已不匹配，删除由 handler 设置的旧值
			ctx.RespHeaders.Del("Content-Length")
		}
	}
}

// shouldCompress 判断给定 Content-Type 是否应进行 gzip 压缩。
// 仅压缩文本类及 JSON/JavaScript/XML 等可压缩类型；
// image/*、video/*、application/zip、application/gzip 等已压缩内容不压缩。
func shouldCompress(contentType string) bool {
	// 去除 charset 等参数部分
	mediaType := strings.TrimSpace(strings.ToLower(strings.Split(contentType, ";")[0]))
	if mediaType == "" {
		return false
	}

	// 文本类一律压缩
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}

	switch mediaType {
	case "application/json",
		"application/javascript",
		"application/x-javascript",
		"text/javascript",
		"application/xml",
		"text/xml",
		"application/xhtml+xml",
		"application/atom+xml",
		"application/rss+xml",
		"application/ld+json",
		"application/manifest+json",
		"application/x-www-form-urlencoded",
		"image/svg+xml":
		return true
	}

	return false
}
