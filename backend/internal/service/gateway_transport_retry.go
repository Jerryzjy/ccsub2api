package service

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
)

// isRetriableTransportError 判断上游传输层错误是否可安全重试。
//
// 仅用于 DoWithTLS 返回 err、尚未拿到任何 HTTP 响应字节的场景：此时请求要么没
// 发出去、要么上游没开始处理，重发是幂等安全的（不会重复计费 / 产生重复副作用）。
//
// 可重试：io.EOF / io.ErrUnexpectedEOF（代理或上游在收到响应前断流）、连接被重置/
// 拒绝/管道破裂等 syscall 级网络错误、以及实现 net.Error 的超时类错误。
//
// 不可重试：context 取消 / 整体超时（客户端主动断或我们自己的 deadline，重试无意义），
// 以及非网络类的普通错误。context 判断优先，避免被 net.Error.Timeout() 误判为可重试。
func isRetriableTransportError(err error) bool {
	if err == nil {
		return false
	}

	// context 取消 / 超时优先短路：这类不是上游连接抖动，重试没有意义。
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// 收到响应前的连接断开。
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// 常见的 syscall 级连接错误。
	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}

	// net 层超时 / 其它网络错误（含代理拨号失败等）。
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	return false
}
