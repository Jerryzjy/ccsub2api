//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newGatewayWithProxyRequire(require bool) *GatewayService {
	return &GatewayService{cfg: &config.Config{
		Proxy: config.ProxyPolicyConfig{RequireForUpstream: require},
	}}
}

func TestEnforceProxyRequirement(t *testing.T) {
	anthropic := &Account{ID: 1, Platform: PlatformAnthropic}
	gemini := &Account{ID: 2, Platform: PlatformGemini}

	t.Run("require on, anthropic, no proxy -> error", func(t *testing.T) {
		s := newGatewayWithProxyRequire(true)
		err := s.enforceProxyRequirement(anthropic, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "refusing direct connection")
	})

	t.Run("require on, anthropic, has proxy -> ok", func(t *testing.T) {
		s := newGatewayWithProxyRequire(true)
		require.NoError(t, s.enforceProxyRequirement(anthropic, "http://proxy:8080"))
	})

	t.Run("require off -> ok even without proxy", func(t *testing.T) {
		s := newGatewayWithProxyRequire(false)
		require.NoError(t, s.enforceProxyRequirement(anthropic, ""))
	})

	t.Run("require on, non-anthropic, no proxy -> ok", func(t *testing.T) {
		s := newGatewayWithProxyRequire(true)
		require.NoError(t, s.enforceProxyRequirement(gemini, ""))
	})

	t.Run("nil account -> ok", func(t *testing.T) {
		s := newGatewayWithProxyRequire(true)
		require.NoError(t, s.enforceProxyRequirement(nil, ""))
	})
}
