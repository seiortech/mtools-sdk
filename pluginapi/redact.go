package pluginapi

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	urlPattern        = regexp.MustCompile(`(?:https?|wss?)://[^\s"'<>]+`)
	authHeaderPattern = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{4,}`)
	kvSecretPattern   = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|api[_-]?key|cookie|authorization|access[_-]?token|refresh[_-]?token|private[_-]?key|credential)\s*([:=])\s*[^\s,;'"&]+`)
)

// IsSensitiveKey reports whether a log attribute key likely holds credentials,
// tokens, cookies, or private secrets that must be redacted.
func IsSensitiveKey(key string) bool {
	norm := strings.ToLower(strings.TrimSpace(key))
	if norm == "" {
		return false
	}
	for _, needle := range []string{
		"password",
		"passwd",
		"secret",
		"token",
		"cookie",
		"authorization",
		"auth",
		"apikey",
		"api_key",
		"private_key",
		"credential",
		"bearer",
		"session",
		"set-cookie",
	} {
		if strings.Contains(norm, needle) {
			return true
		}
	}
	return false
}

// RedactURL strips userinfo, query parameters, and fragments from http(s)/ws(s)
// URLs so tokens or signed URLs are never persisted in logs.
func RedactURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return raw
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ws", "wss":
	default:
		return raw
	}
	hasQuery := u.RawQuery != "" || u.ForceQuery
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	out := u.String()
	if hasQuery {
		out += "?[redacted]"
	}
	return out
}

// RedactString sanitizes a log message or attribute value:
//   - ensures valid UTF-8
//   - strips ANSI escape sequences and control characters (normalizing newlines/tabs)
//   - strips credentials, query strings, and fragments from URLs
//   - masks Bearer/Basic auth tokens and common key=value secret patterns
func RedactString(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ToValidUTF8(s, "")
	s = ansiEscapePattern.ReplaceAllString(s, "")

	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		switch r {
		case '\r', '\n', '\t':
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		default:
			if unicode.IsControl(r) {
				continue
			}
			b.WriteRune(r)
			lastSpace = r == ' '
		}
	}
	clean := strings.TrimSpace(b.String())
	if clean == "" {
		return ""
	}

	clean = urlPattern.ReplaceAllStringFunc(clean, func(match string) string {
		if strings.HasSuffix(match, "?[redacted]") {
			return match
		}
		// Preserve trailing punctuation that is commonly adjacent to prose URLs.
		trimmed := strings.TrimRight(match, ".,);]")
		suffix := match[len(trimmed):]
		return RedactURL(trimmed) + suffix
	})
	clean = authHeaderPattern.ReplaceAllString(clean, "$1 [REDACTED]")
	clean = kvSecretPattern.ReplaceAllString(clean, "$1$2[REDACTED]")
	return clean
}

// TruncateUTF8 truncates s to at most maxBytes without splitting a multi-byte
// UTF-8 rune, appending "…" when truncated and maxBytes >= 4.
func TruncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	const ellipsis = "…"
	limit := maxBytes
	withEllipsis := false
	if maxBytes >= len(ellipsis)+1 {
		limit = maxBytes - len(ellipsis)
		withEllipsis = true
	}
	cut := 0
	for i, r := range s {
		w := utf8.RuneLen(r)
		if w < 0 {
			w = 1
		}
		if i+w > limit {
			break
		}
		cut = i + w
	}
	if withEllipsis {
		return s[:cut] + ellipsis
	}
	return s[:cut]
}
