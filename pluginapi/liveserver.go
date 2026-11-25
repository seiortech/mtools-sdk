package pluginapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// LiveUI is implemented by sidecars that drive a dynamic native UI tree while
// their workspace is open. Open and Event are called sequentially by ServeLive.
type LiveUI interface {
	Open(ctx context.Context, params UIOpenParams, session *UISession) (UITree, error)
	Event(ctx context.Context, event UIEvent, session *UISession) error
}

// UISession lets a LiveUI plugin push updated UI trees, locate its private
// asset directory ($MTOOLS_UI_ASSETS), and access the host's shared tools
// directory and cross-plugin binary resolver.
type UISession struct {
	mu          sync.Mutex
	rev         uint64
	write       func(Message) error
	assets      string
	sharedTools SharedToolsProvider
	logger      *slog.Logger
	levelVar    *slog.LevelVar
}

// Logger returns the structured stderr logger for this Live UI session.
func (s *UISession) Logger() *slog.Logger {
	if s == nil {
		return DefaultLogger()
	}
	s.mu.Lock()
	l := s.logger
	s.mu.Unlock()
	if l == nil {
		return DefaultLogger()
	}
	return l
}

// SetLogLevel updates the session's dynamic log level when backed by ServeLive's
// default stderr logger.
func (s *UISession) SetLogLevel(level string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	lv := s.levelVar
	s.mu.Unlock()
	if lv != nil {
		lv.Set(ParseLogLevel(level))
	}
}

func (s *UISession) setLogger(l *slog.Logger) {
	if s == nil || l == nil {
		return
	}
	s.mu.Lock()
	s.logger = l
	s.mu.Unlock()
}

// AssetsDir returns the host-provided private directory for image assets
// referenced as "asset:<name>".
func (s *UISession) AssetsDir() string {
	return s.assets
}

// SharedTools returns the SharedToolsProvider for this session, backed by the
// host-forwarded settings from ui.open and $MTOOLS_SHARED_TOOLS_DIR.
func (s *UISession) SharedTools() SharedToolsProvider {
	s.mu.Lock()
	st := s.sharedTools
	s.mu.Unlock()
	if st == nil {
		return NewSharedTools(nil)
	}
	return st
}

// SharedToolsDir returns the effective default shared tools directory
// configured by the user in App Settings (default: <PathUserData>/tools).
func (s *UISession) SharedToolsDir() string {
	return s.SharedTools().SharedToolsDir()
}

func (s *UISession) setSharedTools(st SharedToolsProvider) {
	s.mu.Lock()
	s.sharedTools = st
	s.mu.Unlock()
}

// Render validates and pushes a full replacement UITree to the host. If
// tree.Rev is 0, Render assigns the next monotonic revision; otherwise tree.Rev
// must be strictly greater than any previously sent revision.
func (s *UISession) Render(tree UITree) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tree.Rev == 0 {
		tree.Rev = s.rev + 1
	} else if tree.Rev <= s.rev {
		return ErrUITree
	}
	if err := tree.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(tree)
	if err != nil {
		return ErrUITree
	}
	if err := s.write(Message{JSONRPC: "2.0", Method: MethodUITree, Params: raw}); err != nil {
		return err
	}
	s.rev = tree.Rev
	return nil
}

func (s *UISession) adoptOpenTree(tree UITree) (UITree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tree.Rev == 0 {
		tree.Rev = s.rev + 1
	} else if tree.Rev <= s.rev {
		return UITree{}, ErrUITree
	}
	if err := tree.Validate(); err != nil {
		return UITree{}, err
	}
	s.rev = tree.Rev
	return tree, nil
}

// ServeLive runs a jsonrpc-stdio-v1 sidecar that supports both standard
// action execution (Runner / Inspector) and Live UI sessions (LiveUI).
func ServeLive(ctx context.Context, input io.ReadCloser, output io.Writer, identity InitializeResult, runner Runner, live LiveUI) error {
	if err := identity.Validate(); err != nil || live == nil {
		return ErrProtocol
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer input.Close()
	var workers sync.WaitGroup
	defer workers.Wait()
	defer func() { cancel(); input.Close() }()

	type incoming struct {
		m   Message
		err error
	}
	messages := make(chan incoming)
	workers.Go(func() {
		reader := bufio.NewReader(input)
		for {
			m, err := ReadMessage(reader)
			select {
			case messages <- incoming{m, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	})

	var writeMu sync.Mutex
	write := func(m Message) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return WriteMessage(output, m)
	}
	reply := func(id uint64, result any, failure *RPCError) error {
		m := Message{JSONRPC: "2.0", ID: id, Error: failure}
		if failure == nil {
			var err error
			m.Result, err = json.Marshal(result)
			if err != nil {
				return ErrProtocol
			}
		}
		return write(m)
	}

	var levelVar *slog.LevelVar
	baseLogger, hasExplicitLogger := loggerFromContextExplicit(ctx)
	if !hasExplicitLogger {
		levelVar = &slog.LevelVar{}
		levelVar.Set(EffectiveLogLevel())
		baseLogger = NewLogger(os.Stderr, &LogHandlerOptions{Level: levelVar})
	}

	session := &UISession{
		write:       write,
		assets:      os.Getenv("MTOOLS_UI_ASSETS"),
		sharedTools: NewSharedTools(nil),
		logger:      baseLogger,
		levelVar:    levelVar,
	}

	type completion struct {
		id     uint64
		result RunResult
		err    error
	}
	completed := make(chan completion, 1)
	initialized := false
	uiOpened := false
	var activeID string
	var activeCancel context.CancelFunc
	defer func() {
		if activeCancel != nil {
			activeCancel()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case done := <-completed:
			activeCancel()
			activeCancel, activeID = nil, ""
			var failure *RPCError
			if done.err != nil {
				failure = &RPCError{Code: -32000, Message: "Action failed"}
				if errors.Is(done.err, context.Canceled) {
					failure = &RPCError{Code: -32800, Message: "Action canceled"}
				}
			}
			if err := reply(done.id, done.result, failure); err != nil {
				return err
			}
		case in := <-messages:
			if in.err != nil {
				if errors.Is(in.err, io.EOF) {
					return nil
				}
				return in.err
			}
			m := in.m
			if m.Method == "" {
				return ErrProtocol
			}
			switch m.Method {
			case MethodInitialize:
				var p InitializeParams
				if initialized || m.ID == 0 || DecodePayload(m.Params, &p) != nil ||
					p.Validate() != nil || p != identity {
					return ErrProtocol
				}
				if err := reply(m.ID, identity, nil); err != nil {
					return err
				}
				initialized = true
			case MethodUIOpen:
				var p UIOpenParams
				if !initialized || uiOpened || m.ID == 0 || DecodePayload(m.Params, &p) != nil || p.Validate() != nil {
					return ErrProtocol
				}
				session.setSharedTools(NewSharedTools(p.Settings))
				if p.Settings[SettingLogLevel] != "" {
					session.SetLogLevel(p.Settings[SettingLogLevel])
				}
				scopedLogger := baseLogger.With("tool", string(p.Tool))
				session.setLogger(scopedLogger)
				openCtx := WithLogger(ctx, scopedLogger)
				tree, err := live.Open(openCtx, p, session)
				if err != nil {
					scopedLogger.Error("live UI open failed", "err", RedactString(err.Error()))
					if err := reply(m.ID, nil, &RPCError{Code: -32000, Message: "UI open failed"}); err != nil {
						return err
					}
					continue
				}
				tree, err = session.adoptOpenTree(tree)
				if err != nil {
					return err
				}
				if err := reply(m.ID, tree, nil); err != nil {
					return err
				}
				scopedLogger.Debug("live UI opened", "rev", tree.Rev)
				uiOpened = true
			case MethodUIEvent:
				var ev UIEvent
				if !initialized || !uiOpened || m.ID != 0 || DecodePayload(m.Params, &ev) != nil || ev.Validate() != nil {
					return ErrProtocol
				}
				if err := live.Event(WithLogger(ctx, session.Logger()), ev, session); err != nil {
					session.Logger().Error("live UI event failed", "node", ev.Node, "kind", ev.Kind, "err", RedactString(err.Error()))
					return err
				}
			case MethodUIClose:
				if !initialized || m.ID != 0 {
					return ErrProtocol
				}
				session.Logger().Debug("live UI closed")
				return nil
			case MethodInspect:
				inspector, ok := runner.(Inspector)
				if !initialized || !ok || m.ID == 0 {
					if err := reply(m.ID, nil, &RPCError{Code: -32601, Message: "Method not found"}); err != nil {
						return err
					}
					continue
				}
				var p InspectParams
				if DecodePayload(m.Params, &p) != nil || p.Validate() != nil {
					return ErrProtocol
				}
				if p.Settings[SettingLogLevel] != "" {
					session.SetLogLevel(p.Settings[SettingLogLevel])
				}
				inspLogger := baseLogger.With("tool", string(p.Tool))
				res, err := inspector.Inspect(WithLogger(ctx, inspLogger), p)
				var failure *RPCError
				if err != nil {
					inspLogger.Error("inspect failed", "err", RedactString(err.Error()))
					failure = &RPCError{Code: -32000, Message: "Inspect failed"}
				}
				if err := reply(m.ID, res, failure); err != nil {
					return err
				}
			case MethodRun:
				var p RunParams
				if !initialized || runner == nil || activeID != "" || m.ID == 0 ||
					DecodePayload(m.Params, &p) != nil || p.Validate() != nil {
					return ErrProtocol
				}
				if p.Settings[SettingLogLevel] != "" {
					session.SetLogLevel(p.Settings[SettingLogLevel])
				}
				runLogger := baseLogger.With("tool", string(p.Tool), "job_id", p.JobID)
				actionCtx, actionCancel := context.WithCancel(WithLogger(ctx, runLogger))
				activeID, activeCancel = p.JobID, actionCancel
				workers.Go(func() {
					defer func() {
						if r := recover(); r != nil {
							runLogger.Error("plugin action panicked", "panic", RedactString(fmt.Sprint(r)))
							completed <- completion{m.ID, RunResult{}, errors.New("action panicked")}
						}
					}()
					runLogger.Debug("action started", "action", p.Action)
					emit := func(progress Progress) error {
						if err := actionCtx.Err(); err != nil {
							return err
						}
						if err := progress.Validate(p.JobID); err != nil {
							return err
						}
						raw, err := json.Marshal(progress)
						if err != nil {
							return ErrProtocol
						}
						return write(Message{JSONRPC: "2.0", Method: MethodProgress, Params: raw})
					}
					result, err := runner.Run(actionCtx, p, emit)
					if err == nil {
						err = actionCtx.Err()
					}
					if err == nil {
						err = result.Validate(p.JobID)
					}
					switch {
					case err == nil:
						runLogger.Debug("action completed", "action", p.Action, "outputs", len(result.Outputs))
					case errors.Is(err, context.Canceled):
						runLogger.Info("action canceled", "action", p.Action)
					default:
						runLogger.Error("action failed", "action", p.Action, "err", RedactString(err.Error()))
					}
					completed <- completion{m.ID, result, err}
				})
			case MethodCancel:
				var p CancelParams
				if !initialized || m.ID != 0 || DecodePayload(m.Params, &p) != nil ||
					!ValidJobID(p.JobID) || p.JobID != activeID {
					return ErrProtocol
				}
				activeCancel()
			default:
				if !initialized || m.ID == 0 {
					return ErrProtocol
				}
				if err := reply(m.ID, nil, &RPCError{Code: -32601, Message: "Method not found"}); err != nil {
					return err
				}
			}
		}
	}
}
