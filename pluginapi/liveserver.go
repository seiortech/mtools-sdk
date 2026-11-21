package pluginapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
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

	session := &UISession{
		write:       write,
		assets:      os.Getenv("MTOOLS_UI_ASSETS"),
		sharedTools: NewSharedTools(nil),
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
				tree, err := live.Open(ctx, p, session)
				if err != nil {
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
				uiOpened = true
			case MethodUIEvent:
				var ev UIEvent
				if !initialized || !uiOpened || m.ID != 0 || DecodePayload(m.Params, &ev) != nil || ev.Validate() != nil {
					return ErrProtocol
				}
				if err := live.Event(ctx, ev, session); err != nil {
					return err
				}
			case MethodUIClose:
				if !initialized || m.ID != 0 {
					return ErrProtocol
				}
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
				res, err := inspector.Inspect(ctx, p)
				var failure *RPCError
				if err != nil {
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
				actionCtx, actionCancel := context.WithCancel(ctx)
				activeID, activeCancel = p.JobID, actionCancel
				workers.Go(func() {
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
