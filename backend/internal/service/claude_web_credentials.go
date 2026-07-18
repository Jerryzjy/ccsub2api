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
	ClaudeWebCredentialEmailAddress   = "email_address"
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
	if strings.TrimSpace(cookie) != "" {
		normalized, err := NormalizeClaudeWebCookie(cookie, time.Now())
		if err != nil {
			return fmt.Errorf("invalid cookie: %w", err)
		}
		if normalized.SessionKey == "" && strings.TrimSpace(sessionKey) == "" {
			return errors.New("cookie does not contain sessionKey")
		}
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

func mergeClaudeWebSessionCredentials(existing, incoming map[string]any) map[string]any {
	merged := MergePreservingSensitiveCreds(existing, incoming)
	cookieReplaced := credentialString(incoming, ClaudeWebCredentialCookie) != ""
	sessionKeyReplaced := credentialString(incoming, ClaudeWebCredentialSessionKey) != ""
	if !cookieReplaced && !sessionKeyReplaced {
		return merged
	}

	// organization_id and email_address are derived from the authenticated
	// Claude account. Keeping them across an auth replacement can route the new
	// session to the previous account's organization and stale profile.
	delete(merged, ClaudeWebCredentialOrganizationID)
	delete(merged, ClaudeWebCredentialEmailAddress)

	// A full Cookie takes precedence during request construction. When callers
	// replace only one auth form, remove the other form so an older value cannot
	// silently win over the newly supplied credential.
	if cookieReplaced && !sessionKeyReplaced {
		delete(merged, ClaudeWebCredentialSessionKey)
	}
	if sessionKeyReplaced && !cookieReplaced {
		delete(merged, ClaudeWebCredentialCookie)
	}
	return merged
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
		line, skip := normalizeNetscapeCookieLine(line)
		if skip {
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
		line, skip := normalizeNetscapeCookieLine(line)
		if skip {
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

func normalizeNetscapeCookieLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", true
	}
	if strings.HasPrefix(trimmed, "#HttpOnly_") {
		return strings.TrimPrefix(trimmed, "#HttpOnly_"), false
	}
	if strings.HasPrefix(trimmed, "#") {
		return "", true
	}
	return line, false
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
