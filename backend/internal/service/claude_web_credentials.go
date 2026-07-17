package service

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ClaudeWebCredentialCookie         = "cookie"
	ClaudeWebCredentialSessionKey     = "session_key"
	ClaudeWebCredentialOrganizationID = "organization_id"
)

// ClaudeWebCookie is the normalized subset needed by the Claude Web transport.
type ClaudeWebCookie struct {
	Header         string
	SessionKey     string
	OrganizationID string
}

type claudeWebCookieEntry struct {
	name    string
	value   string
	domain  string
	path    string
	expires int64
}

// ValidateClaudeWebSessionCredentials validates the stable account contract.
// Cookie values are deliberately excluded from returned errors.
func ValidateClaudeWebSessionCredentials(platform string, credentials map[string]any) error {
	if platform != PlatformAnthropic {
		return errors.New("web_session is only supported for anthropic")
	}
	cookie := credentialString(credentials, ClaudeWebCredentialCookie)
	sessionKey := credentialString(credentials, ClaudeWebCredentialSessionKey)
	if strings.TrimSpace(cookie) == "" && strings.TrimSpace(sessionKey) == "" {
		return errors.New("cookie or session_key is required")
	}
	return nil
}

func credentialString(credentials map[string]any, key string) string {
	if credentials == nil {
		return ""
	}
	value, _ := credentials[key].(string)
	return strings.TrimSpace(value)
}

// NormalizeClaudeWebCookie accepts either a Cookie request header or a
// Netscape cookie export and returns a deterministic Cookie header.
func NormalizeClaudeWebCookie(raw string, now time.Time) (ClaudeWebCookie, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ClaudeWebCookie{}, errors.New("cookie input is empty")
	}

	entries, netscape, err := parseClaudeWebCookieInput(raw, now)
	if err != nil {
		return ClaudeWebCookie{}, err
	}
	if len(entries) == 0 {
		if netscape {
			return ClaudeWebCookie{}, errors.New("no claude.ai cookies found")
		}
		return ClaudeWebCookie{}, errors.New("no valid cookies found")
	}

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		entry := entries[name]
		parts = append(parts, entry.name+"="+entry.value)
	}

	result := ClaudeWebCookie{Header: strings.Join(parts, "; ")}
	if entry, ok := entries["sessionKey"]; ok {
		result.SessionKey = entry.value
	}
	if entry, ok := entries["lastActiveOrg"]; ok {
		result.OrganizationID = entry.value
	}
	return result, nil
}

func parseClaudeWebCookieInput(raw string, now time.Time) (map[string]claudeWebCookieEntry, bool, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	netscape := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Count(line, "\t") >= 6 {
			netscape = true
		}
		break
	}
	if !netscape {
		entries, err := parseCookieHeader(raw)
		return entries, false, err
	}

	entries := make(map[string]claudeWebCookieEntry)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.SplitN(line, "\t", 7)
		if len(fields) != 7 {
			return nil, true, errors.New("invalid Netscape cookie line")
		}
		domain := strings.ToLower(strings.TrimSpace(fields[0]))
		if !isClaudeWebCookieDomain(domain) {
			continue
		}
		expires, err := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		if err != nil {
			return nil, true, errors.New("invalid Netscape cookie expiry")
		}
		if expires > 0 && expires <= now.Unix() {
			continue
		}
		name := strings.TrimSpace(fields[5])
		if name == "" {
			continue
		}
		candidate := claudeWebCookieEntry{
			name:    name,
			value:   strings.TrimSpace(fields[6]),
			domain:  domain,
			path:    strings.TrimSpace(fields[2]),
			expires: expires,
		}
		if current, exists := entries[name]; !exists || preferClaudeWebCookie(candidate, current) {
			entries[name] = candidate
		}
	}
	return entries, true, nil
}

func parseCookieHeader(raw string) (map[string]claudeWebCookieEntry, error) {
	entries := make(map[string]claudeWebCookieEntry)
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid Cookie header entry")
		}
		entries[name] = claudeWebCookieEntry{name: name, value: strings.TrimSpace(value), path: "/"}
	}
	return entries, nil
}

func isClaudeWebCookieDomain(domain string) bool {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
	return domain == "claude.ai" || strings.HasSuffix(domain, ".claude.ai")
}

func preferClaudeWebCookie(candidate, current claudeWebCookieEntry) bool {
	if candidate.path == "/" && current.path != "/" {
		return true
	}
	if candidate.path != "/" && current.path == "/" {
		return false
	}
	if candidate.domain == ".claude.ai" && current.domain != ".claude.ai" {
		return true
	}
	return candidate.expires > current.expires
}
