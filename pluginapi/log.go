package pluginapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// EnvLogLevel is injected by the host runtime when launching sidecars.
	EnvLogLevel = "MTOOLS_LOG_LEVEL"

	// SettingLogLevel is forwarded in RunParams/InspectParams/UIOpenParams Settings.
	SettingLogLevel = "log_level"

	LogWireSchema = 1
	LogWireMarker = "1"

	MaxLogLineBytes   = 4096
	MaxLogMessageSize = 1024
	MaxLogAttrs       = 32
	MaxLogAttrKey     = 64
	MaxLogAttrValue   = 512
)

// LogLevel is the canonical wire log severity.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogEntry is the structured NDJSON envelope emitted by pluginapi.Logger to stderr.
type LogEntry struct {
	Schema    int               `json:"v"`
	Marker    string            `json:"mtools_log"`
	Time      string            `json:"ts"`
	Level     LogLevel          `json:"level"`
	Message   string            `json:"msg"`
	Component string            `json:"component,omitempty"`
	Tool      ToolID            `json:"tool,omitempty"`
	JobID     string            `json:"job_id,omitempty"`
	Attrs     map[string]string `json:"attrs,omitempty"`
}

// LogHandlerOptions configures a structured stderr slog.Handler for plugins.
type LogHandlerOptions struct {
	Level        slog.Leveler // nil defaults to EffectiveLogLevel()
	Component    string
	Tool         ToolID
	JobID        string
	MaxLineBytes int // <=0 defaults to MaxLogLineBytes
}

// ParseLogLevel converts a host/env log level string to slog.Level (defaulting to Info).
func ParseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NormalizeLogLevel returns a canonical LogLevel string (defaulting to info).
func NormalizeLogLevel(raw string) LogLevel {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "warn", "warning":
		return LogLevelWarn
	case "error":
		return LogLevelError
	default:
		return LogLevelInfo
	}
}

// FormatLogLevel maps an slog.Level to the canonical wire LogLevel.
func FormatLogLevel(l slog.Level) LogLevel {
	switch {
	case l < slog.LevelInfo:
		return LogLevelDebug
	case l < slog.LevelWarn:
		return LogLevelInfo
	case l < slog.LevelError:
		return LogLevelWarn
	default:
		return LogLevelError
	}
}

// EffectiveLogLevel resolves the active log level from $MTOOLS_LOG_LEVEL.
func EffectiveLogLevel() slog.Level {
	return ParseLogLevel(os.Getenv(EnvLogLevel))
}

type logHandler struct {
	mu           *sync.Mutex
	w            io.Writer
	level        slog.Leveler
	component    string
	tool         ToolID
	jobID        string
	maxLineBytes int
	groups       []string
	attrs        []slog.Attr
}

// NewLogHandler returns an slog.Handler that writes redacted, bounded NDJSON
// LogEntry lines. It defaults w to os.Stderr (never os.Stdout).
func NewLogHandler(w io.Writer, opts *LogHandlerOptions) slog.Handler {
	if w == nil {
		w = os.Stderr
	}
	var cfg LogHandlerOptions
	if opts != nil {
		cfg = *opts
	}
	if cfg.Level == nil {
		cfg.Level = EffectiveLogLevel()
	}
	if cfg.MaxLineBytes <= 0 {
		cfg.MaxLineBytes = MaxLogLineBytes
	}
	return &logHandler{
		mu:           &sync.Mutex{},
		w:            w,
		level:        cfg.Level,
		component:    TruncateUTF8(RedactString(cfg.Component), MaxLogAttrKey),
		tool:         cfg.Tool,
		jobID:        TruncateUTF8(RedactString(cfg.JobID), MaxLogAttrKey),
		maxLineBytes: cfg.MaxLineBytes,
	}
}

// NewLogger constructs a structured *slog.Logger backed by NewLogHandler.
func NewLogger(w io.Writer, opts *LogHandlerOptions) *slog.Logger {
	return slog.New(NewLogHandler(w, opts))
}

// DefaultLogger returns a structured logger writing to os.Stderr at EffectiveLogLevel().
func DefaultLogger() *slog.Logger {
	return NewLogger(os.Stderr, nil)
}

type loggerContextKey struct{}

// WithLogger attaches logger to ctx for retrieval via LoggerFromContext.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

func loggerFromContextExplicit(ctx context.Context) (*slog.Logger, bool) {
	if ctx == nil {
		return nil, false
	}
	l, ok := ctx.Value(loggerContextKey{}).(*slog.Logger)
	return l, ok && l != nil
}

// LoggerFromContext returns the logger attached to ctx, or DefaultLogger().
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := loggerFromContextExplicit(ctx); ok {
		return l
	}
	return DefaultLogger()
}

func (h *logHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.level != nil {
		minLevel = h.level.Level()
	}
	return level >= minLevel
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	cloned := *h
	combined := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	combined = append(combined, h.attrs...)
	for _, a := range attrs {
		combined = append(combined, prefixGroupAttr(h.groups, a))
	}
	cloned.attrs = combined
	return &cloned
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	name = strings.TrimSpace(name)
	if name == "" {
		return h
	}
	cloned := *h
	groups := make([]string, len(h.groups)+1)
	copy(groups, h.groups)
	groups[len(h.groups)] = name
	cloned.groups = groups
	return &cloned
}

func prefixGroupAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 || a.Key == "" {
		return a
	}
	a.Key = strings.Join(groups, ".") + "." + a.Key
	return a
}

func (h *logHandler) Handle(_ context.Context, rec slog.Record) error {
	ts := rec.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	entry := LogEntry{
		Schema:    LogWireSchema,
		Marker:    LogWireMarker,
		Time:      ts.UTC().Format(time.RFC3339Nano),
		Level:     FormatLogLevel(rec.Level),
		Message:   TruncateUTF8(RedactString(rec.Message), MaxLogMessageSize),
		Component: h.component,
		Tool:      h.tool,
		JobID:     h.jobID,
	}

	attrs := make(map[string]string)
	applyAttr := func(k string, v slog.Value) {
		k = TruncateUTF8(RedactString(k), MaxLogAttrKey)
		if k == "" {
			return
		}
		switch k {
		case "component":
			entry.Component = TruncateUTF8(RedactString(v.String()), MaxLogAttrKey)
			return
		case "tool":
			t := ToolID(TruncateUTF8(RedactString(v.String()), MaxLogAttrKey))
			if t != "" {
				entry.Tool = t
			}
			return
		case "job_id":
			entry.JobID = TruncateUTF8(RedactString(v.String()), MaxLogAttrKey)
			return
		}
		if len(attrs) >= MaxLogAttrs {
			if _, exists := attrs[k]; !exists {
				return
			}
		}
		if IsSensitiveKey(k) {
			attrs[k] = "[REDACTED]"
			return
		}
		attrs[k] = TruncateUTF8(RedactString(v.String()), MaxLogAttrValue)
	}

	var walkAttr func(prefix string, a slog.Attr)
	walkAttr = func(prefix string, a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			return
		}
		key := a.Key
		if prefix != "" {
			if key != "" {
				key = prefix + "." + key
			} else {
				key = prefix
			}
		}
		if a.Value.Kind() == slog.KindGroup {
			for _, sub := range a.Value.Group() {
				walkAttr(key, sub)
			}
			return
		}
		if key != "" {
			applyAttr(key, a.Value)
		}
	}

	for _, a := range h.attrs {
		walkAttr("", a)
	}
	recPrefix := strings.Join(h.groups, ".")
	rec.Attrs(func(a slog.Attr) bool {
		walkAttr(recPrefix, a)
		return true
	})
	if len(attrs) > 0 {
		entry.Attrs = attrs
	}

	line, err := encodeBoundedLogEntry(entry, h.maxLineBytes)
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.w.Write(line)
	return err
}

func encodeBoundedLogEntry(entry LogEntry, maxLineBytes int) ([]byte, error) {
	if maxLineBytes <= 0 {
		maxLineBytes = MaxLogLineBytes
	}
	marshalLine := func(e LogEntry) ([]byte, error) {
		raw, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		return append(raw, '\n'), nil
	}
	line, err := marshalLine(entry)
	if err != nil {
		return nil, err
	}
	if len(line) <= maxLineBytes {
		return line, nil
	}
	// Progressively drop attributes, then shrink message so the frame fits maxLineBytes.
	if len(entry.Attrs) > 0 {
		entry.Attrs = nil
		line, err = marshalLine(entry)
		if err != nil || len(line) <= maxLineBytes {
			return line, err
		}
	}
	for limit := len(entry.Message) / 2; limit >= 16; limit /= 2 {
		entry.Message = TruncateUTF8(entry.Message, limit)
		line, err = marshalLine(entry)
		if err != nil || len(line) <= maxLineBytes {
			return line, err
		}
	}
	entry.Message = ""
	entry.Component = ""
	entry.JobID = ""
	return marshalLine(entry)
}

// DecodeLogEntry parses a single NDJSON stderr line emitted by pluginapi.Logger.
// Returns (entry, true) only when the schema and marker match LogEntry.
func DecodeLogEntry(line []byte) (LogEntry, bool) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 || len(trimmed) > MaxLogLineBytes || trimmed[0] != '{' {
		return LogEntry{}, false
	}
	var entry LogEntry
	if err := json.Unmarshal(trimmed, &entry); err != nil {
		return LogEntry{}, false
	}
	if entry.Schema != LogWireSchema || entry.Marker != LogWireMarker {
		return LogEntry{}, false
	}
	switch entry.Level {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
	default:
		return LogEntry{}, false
	}
	if entry.Message == "" && len(entry.Attrs) == 0 {
		return LogEntry{}, false
	}
	entry.Message = TruncateUTF8(RedactString(entry.Message), MaxLogMessageSize)
	entry.Component = TruncateUTF8(RedactString(entry.Component), MaxLogAttrKey)
	entry.JobID = TruncateUTF8(RedactString(entry.JobID), MaxLogAttrKey)
	if len(entry.Attrs) > 0 {
		cleanAttrs := make(map[string]string, min(len(entry.Attrs), MaxLogAttrs))
		for k, v := range entry.Attrs {
			if len(cleanAttrs) >= MaxLogAttrs {
				break
			}
			ck := TruncateUTF8(RedactString(k), MaxLogAttrKey)
			if ck == "" {
				continue
			}
			if IsSensitiveKey(ck) {
				cleanAttrs[ck] = "[REDACTED]"
			} else {
				cleanAttrs[ck] = TruncateUTF8(RedactString(v), MaxLogAttrValue)
			}
		}
		if len(cleanAttrs) > 0 {
			entry.Attrs = cleanAttrs
		} else {
			entry.Attrs = nil
		}
	}
	return entry, true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// String helper for debugging; keeps fmt import unnecessary elsewhere.
func (e LogEntry) String() string {
	return fmt.Sprintf("[%s] %s", e.Level, e.Message)
}
