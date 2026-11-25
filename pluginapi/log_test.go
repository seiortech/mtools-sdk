package pluginapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRedactionHelpers(t *testing.T) {
	if !IsSensitiveKey("API_Key") || !IsSensitiveKey("authorization") || !IsSensitiveKey("access_token") {
		t.Fatal("expected sensitive keys to be detected")
	}
	if IsSensitiveKey("file_name") || IsSensitiveKey("duration_ms") {
		t.Fatal("unexpected sensitive key match")
	}

	gotURL := RedactURL("https://user:secret@example.com/download?token=abc&sig=123#frag")
	if strings.Contains(gotURL, "user") || strings.Contains(gotURL, "secret") ||
		strings.Contains(gotURL, "abc") || strings.Contains(gotURL, "frag") {
		t.Fatalf("URL not redacted: %q", gotURL)
	}
	if gotURL != "https://example.com/download?[redacted]" {
		t.Fatalf("unexpected redacted URL: %q", gotURL)
	}

	raw := "\x1b[31merror\x1b[0m\nAuthorization: Bearer my-secret-jwt-token token=xyz123 https://u:p@host.test/v?k=v"
	redacted := RedactString(raw)
	for _, leak := range []string{"\x1b", "\n", "my-secret-jwt-token", "xyz123", "u:p", "k=v"} {
		if strings.Contains(redacted, leak) {
			t.Fatalf("redacted string still contains %q: %q", leak, redacted)
		}
	}

	trunc := TruncateUTF8("こんにちは世界", 10)
	if !utf8.ValidString(trunc) || len(trunc) > 10 || !strings.HasSuffix(trunc, "…") {
		t.Fatalf("invalid UTF-8 truncation: %q (len=%d)", trunc, len(trunc))
	}
}

func TestLogHandlerAndDecodeLogEntry(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	safeBuf := &syncWriter{mu: &mu, w: &buf}

	logger := NewLogger(safeBuf, &LogHandlerOptions{
		Level: slog.LevelDebug,
		Tool:  "echo",
	}).With("component", "engine").WithGroup("req").With("token", "should-redact", "url", "https://example.com/x?a=1")

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Info("processing item", "idx", n, "big", strings.Repeat("z", 200))
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 8 {
		t.Fatalf("expected 8 NDJSON lines, got %d", len(lines))
	}
	for _, line := range lines {
		entry, ok := DecodeLogEntry([]byte(line))
		if !ok {
			t.Fatalf("DecodeLogEntry rejected valid line: %s", line)
		}
		if entry.Component != "engine" || entry.Tool != "echo" {
			t.Fatalf("unexpected metadata: %+v", entry)
		}
		if entry.Attrs["req.token"] != "[REDACTED]" {
			t.Fatalf("expected req.token redacted, got %q", entry.Attrs["req.token"])
		}
		if entry.Attrs["req.url"] != "https://example.com/x?[redacted]" {
			t.Fatalf("expected req.url redacted, got %q", entry.Attrs["req.url"])
		}
	}

	// Oversize payload shrinks cleanly within MaxLineBytes.
	var smallBuf bytes.Buffer
	smallLogger := NewLogger(&smallBuf, &LogHandlerOptions{MaxLineBytes: 220})
	smallLogger.Info(strings.Repeat("m", 500), "extra", strings.Repeat("x", 300))
	if smallBuf.Len() > 220 {
		t.Fatalf("expected line <= 220 bytes, got %d: %s", smallBuf.Len(), smallBuf.String())
	}
	if _, ok := DecodeLogEntry(smallBuf.Bytes()); !ok {
		t.Fatalf("shrunk line failed DecodeLogEntry: %s", smallBuf.String())
	}

	// Non-mtools JSON must be rejected.
	if _, ok := DecodeLogEntry([]byte(`{"level":"info","msg":"foreign"}`)); ok {
		t.Fatal("expected foreign JSON without mtools_log marker to be rejected")
	}
}

func TestServeAndServeLiveContextLogging(t *testing.T) {
	t.Run("Serve", func(t *testing.T) {
		host, child := net.Pipe()
		defer host.Close()
		host.SetDeadline(time.Now().Add(5 * time.Second))

		var logBuf bytes.Buffer
		var logMu sync.Mutex
		ctx := WithLogger(t.Context(), NewLogger(&syncWriter{mu: &logMu, w: &logBuf}, &LogHandlerOptions{
			Level: slog.LevelDebug,
		}))

		identity := InitializeResult{Protocol: Protocol, PluginID: "org.example.echo", Version: "1.0.0"}
		finished := make(chan error, 1)
		go func() {
			finished <- Serve(ctx, child, child, identity, RunnerFunc(func(ctx context.Context, p RunParams, emit func(Progress) error) (RunResult, error) {
				LoggerFromContext(ctx).Info("inside runner", "component", "test_runner", "secret", "top-secret")
				return RunResult{JobID: p.JobID, Outputs: []string{}, Summary: "ok"}, nil
			}))
		}()

		send := func(id uint64, method string, p any) {
			raw, _ := json.Marshal(p)
			if err := WriteMessage(host, Message{JSONRPC: "2.0", ID: id, Method: method, Params: raw}); err != nil {
				t.Fatal(err)
			}
		}
		reader := bufio.NewReader(host)
		send(1, MethodInitialize, identity)
		if _, err := ReadMessage(reader); err != nil {
			t.Fatal(err)
		}
		send(2, MethodRun, RunParams{JobID: "run-1", Tool: "echo", Action: "echo", Inputs: map[string][]string{}, Settings: map[string]string{}})
		if m, err := ReadMessage(reader); err != nil || m.ID != 2 || m.Error != nil {
			t.Fatalf("run reply: %+v err=%v", m, err)
		}
		host.Close()
		if err := <-finished; err != nil {
			t.Fatal(err)
		}

		logMu.Lock()
		logged := logBuf.String()
		logMu.Unlock()
		if !strings.Contains(logged, "inside runner") || strings.Contains(logged, "top-secret") {
			t.Fatalf("unexpected captured logs: %s", logged)
		}
	})

	t.Run("ServeLive", func(t *testing.T) {
		inRead, inWrite := io.Pipe()
		outRead, outWrite := io.Pipe()

		var logBuf bytes.Buffer
		var logMu sync.Mutex
		ctx := WithLogger(t.Context(), NewLogger(&syncWriter{mu: &logMu, w: &logBuf}, &LogHandlerOptions{
			Level: slog.LevelDebug,
		}))

		id := InitializeResult{Protocol: Protocol, PluginID: "org.example.live", Version: "1.0.0"}
		live := &loggingLiveFixture{}
		done := make(chan error, 1)
		go func() {
			done <- ServeLive(ctx, inRead, outWrite, id, nil, live)
		}()

		send := func(m Message) {
			if err := WriteMessage(inWrite, m); err != nil {
				t.Fatal(err)
			}
		}
		reader := bufio.NewReader(outRead)
		initParams, _ := json.Marshal(id)
		send(Message{JSONRPC: "2.0", ID: 1, Method: MethodInitialize, Params: initParams})
		_, _ = ReadMessage(reader)

		openParams, _ := json.Marshal(UIOpenParams{Tool: "live", Viewport: UIViewport{Width: 800, Height: 600}})
		send(Message{JSONRPC: "2.0", ID: 2, Method: MethodUIOpen, Params: openParams})
		if r2, err := ReadMessage(reader); err != nil || r2.Error != nil {
			t.Fatalf("open reply: %+v err=%v", r2, err)
		}

		closeParams, _ := json.Marshal(UICloseParams{Tool: "live"})
		send(Message{JSONRPC: "2.0", Method: MethodUIClose, Params: closeParams})
		if err := <-done; err != nil {
			t.Fatal(err)
		}

		logMu.Lock()
		logged := logBuf.String()
		logMu.Unlock()
		if !strings.Contains(logged, "live open called") {
			t.Fatalf("expected live open log, got: %s", logged)
		}
	})
}

type loggingLiveFixture struct{}

func (l *loggingLiveFixture) Open(_ context.Context, _ UIOpenParams, s *UISession) (UITree, error) {
	s.Logger().Info("live open called", "component", "live_fixture")
	return UITree{Root: UINode{ID: "root", Kind: UINodeColumn}}, nil
}

func (l *loggingLiveFixture) Event(_ context.Context, _ UIEvent, _ *UISession) error {
	return nil
}

type syncWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
