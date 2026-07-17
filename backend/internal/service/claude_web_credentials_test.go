package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateClaudeWebSessionCredentials(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		credentials map[string]any
		wantErr     string
	}{
		{
			name:        "full cookie",
			platform:    PlatformAnthropic,
			credentials: map[string]any{"cookie": "sessionKey=abc"},
		},
		{
			name:        "session key fallback",
			platform:    PlatformAnthropic,
			credentials: map[string]any{"session_key": "sk-ant-sid01-test"},
		},
		{
			name:        "wrong platform",
			platform:    PlatformOpenAI,
			credentials: map[string]any{"cookie": "sessionKey=abc"},
			wantErr:     "web_session is only supported for anthropic",
		},
		{
			name:        "missing credential",
			platform:    PlatformAnthropic,
			credentials: map[string]any{},
			wantErr:     "cookie or session_key is required",
		},
		{
			name:        "blank credential",
			platform:    PlatformAnthropic,
			credentials: map[string]any{"cookie": "  ", "session_key": "\t"},
			wantErr:     "cookie or session_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateClaudeWebSessionCredentials(tt.platform, tt.credentials)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestAccountIsClaudeWebSession(t *testing.T) {
	require.True(t, (&Account{Platform: PlatformAnthropic, Type: AccountTypeWebSession}).IsClaudeWebSession())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeWebSession}).IsClaudeWebSession())
	require.False(t, (&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}).IsClaudeWebSession())
}

func TestNormalizeClaudeWebCookie_Header(t *testing.T) {
	got, err := NormalizeClaudeWebCookie(
		"foo=bar; sessionKey=cookie-value; lastActiveOrg=org-1",
		time.Unix(1_800_000_000, 0),
	)

	require.NoError(t, err)
	require.Equal(t, "foo=bar; lastActiveOrg=org-1; sessionKey=cookie-value", got.Header)
	require.Equal(t, "cookie-value", got.SessionKey)
	require.Equal(t, "org-1", got.OrganizationID)
}

func TestNormalizeClaudeWebCookie_Netscape(t *testing.T) {
	raw := "# Netscape HTTP Cookie File\n" +
		".claude.ai\tTRUE\t/\tTRUE\t4102444800\tsessionKey\tsk-test\n" +
		".claude.ai\tTRUE\t/\tTRUE\t4102444800\tlastActiveOrg\torg-2\n" +
		"example.com\tTRUE\t/\tTRUE\t4102444800\tignored\tx\n"

	got, err := NormalizeClaudeWebCookie(raw, time.Unix(1_800_000_000, 0))

	require.NoError(t, err)
	require.Equal(t, "lastActiveOrg=org-2; sessionKey=sk-test", got.Header)
	require.Equal(t, "sk-test", got.SessionKey)
	require.Equal(t, "org-2", got.OrganizationID)
}

func TestNormalizeClaudeWebCookie_NetscapeSkipsExpiredAndPrefersRootPath(t *testing.T) {
	raw := ".claude.ai\tTRUE\t/chat\tTRUE\t4102444800\tsessionKey\tchat-value\n" +
		".claude.ai\tTRUE\t/\tTRUE\t4102444800\tsessionKey\troot-value\n" +
		".claude.ai\tTRUE\t/\tTRUE\t1700000000\told\texpired\n"

	got, err := NormalizeClaudeWebCookie(raw, time.Unix(1_800_000_000, 0))

	require.NoError(t, err)
	require.Equal(t, "sessionKey=root-value", got.Header)
	require.Equal(t, "root-value", got.SessionKey)
}

func TestNormalizeClaudeWebCookie_RejectsInputWithoutClaudeCookies(t *testing.T) {
	_, err := NormalizeClaudeWebCookie(
		"example.com\tTRUE\t/\tTRUE\t4102444800\tfoo\tbar\n",
		time.Unix(1_800_000_000, 0),
	)

	require.EqualError(t, err, "no claude.ai cookies found")
}
