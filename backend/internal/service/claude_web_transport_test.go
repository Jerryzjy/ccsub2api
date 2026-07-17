package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaudeWebBrowserTransportDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, ClaudeWebUserAgent, r.Header.Get("User-Agent"))
		require.Equal(t, "sessionKey=test", r.Header.Get("Cookie"))
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer server.Close()

	transport := NewClaudeWebBrowserTransport()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/test", nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", ClaudeWebUserAgent)
	req.Header.Set("Cookie", "sessionKey=test")

	resp, err := transport.Do(context.Background(), req, "", 42)

	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Equal(t, "ok", resp.Header.Get("X-Test"))
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "created", string(body))
}

func TestClaudeWebBrowserTransportReusesClientByAccountAndProxy(t *testing.T) {
	transport := NewClaudeWebBrowserTransport()

	require.Same(t, transport.clientSlot(1, ""), transport.clientSlot(1, ""))
	require.NotSame(t, transport.clientSlot(1, ""), transport.clientSlot(2, ""))
	require.NotSame(t, transport.clientSlot(1, ""), transport.clientSlot(1, "http://proxy.example:8080"))
}
