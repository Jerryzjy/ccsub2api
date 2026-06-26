package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
)

// isRetriableTransportError 判断上游传输层错误（DoWithTLS 返回 err、尚未拿到 HTTP
// 响应）是否可安全重试。代理偶发断流产生的 EOF / connection reset 属于此类——
// 请求未被上游处理，重试幂等安全。context 取消 / 普通错误不重试。

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "i/o timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return true }

func TestIsRetriableTransportError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"io.EOF", io.EOF, true},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"wrapped EOF", fmt.Errorf("Post ...: %w", io.EOF), true},
		{"connection reset", syscall.ECONNRESET, true},
		{"broken pipe", syscall.EPIPE, true},
		{"connection refused", syscall.ECONNREFUSED, true},
		{"net timeout", timeoutNetErr{}, true},
		{"wrapped net.OpError EOF", &net.OpError{Op: "read", Err: io.EOF}, true},

		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		{"plain error", errors.New("some logic error"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetriableTransportError(tc.err); got != tc.want {
				t.Errorf("isRetriableTransportError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
