package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestValidateDataAccount_ClaudeWebSession(t *testing.T) {
	t.Run("accepts anthropic cookie", func(t *testing.T) {
		err := validateDataAccount(DataAccount{
			Name:     "Claude Web",
			Platform: service.PlatformAnthropic,
			Type:     service.AccountTypeWebSession,
			Credentials: map[string]any{
				"cookie": "sessionKey=test",
			},
		})
		require.NoError(t, err)
	})

	t.Run("rejects wrong platform", func(t *testing.T) {
		err := validateDataAccount(DataAccount{
			Name:     "Wrong",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeWebSession,
			Credentials: map[string]any{
				"cookie": "sessionKey=test",
			},
		})
		require.EqualError(t, err, "web_session is only supported for anthropic")
	})

	t.Run("rejects missing credential", func(t *testing.T) {
		err := validateDataAccount(DataAccount{
			Name:        "Missing",
			Platform:    service.PlatformAnthropic,
			Type:        service.AccountTypeWebSession,
			Credentials: map[string]any{"other": "value"},
		})
		require.EqualError(t, err, "cookie or session_key is required")
	})
}
