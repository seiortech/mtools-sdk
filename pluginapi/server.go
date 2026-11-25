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

// Runner handles one action at a time. It must honor ctx and finish before
// returning. emit is safe from multiple goroutines, but must not be retained.
type Runner interface {
	Run(context.Context, RunParams, func(Progress) error) (RunResult, error)
}

type RunnerFunc func(context.Context, RunParams, func(Progress) error) (RunResult, error)

func (f RunnerFunc) Run(ctx context.Context, p RunParams, emit func(Progress) error) (RunResult, error) {
	return f(ctx, p, emit)
}

// Inspector is an optional interface a Runner may implement to support dynamic
// schema inspection, previews, and helper binary checks.
type Inspector interface {
	Inspect(context.Context, InspectParams) (InspectResult, error)
}

type InspectorFunc func(context.Context, InspectParams) (InspectResult, error)

func (f InspectorFunc) Inspect(ctx context.Context, p InspectParams) (InspectResult, error) {
	return f(ctx, p)
}

// Serve is the optional Go sidecar SDK, independent of the desktop host.
// It owns and closes input so cancellation can interrupt a blocked read.
// output must be protocol-only and must not block forever (the host closes it
// on termination). Any diagnostics belong on stderr, never output.
func Serve(ctx context.Context, input io.ReadCloser, output io.Writer, identity InitializeResult, runner Runner) error {
	if err := identity.Validate(); err != nil || runner == nil {
		return ErrProtocol
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer input.Close()
	var workers sync.WaitGroup
	defer workers.Wait()
	// Cancel and close input before waiting for the action and reader.
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
	type completion struct {
		id     uint64
		result RunResult
		err    error
	}
	completed := make(chan completion, 1)
	initialized := false
	var levelVar *slog.LevelVar
	baseLogger, hasExplicitLogger := loggerFromContextExplicit(ctx)
	if !hasExplicitLogger {
		levelVar = &slog.LevelVar{}
		levelVar.Set(EffectiveLogLevel())
		baseLogger = NewLogger(os.Stderr, &LogHandlerOptions{Level: levelVar})
	}
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
				// Do not leak raw errors containing filenames, URLs or secrets.
				failure = &RPCError{Code: -32000, Message: "Action failed"}
				if errors.Is(done.err, context.Canceled) {
					failure = &RPCError{Code: -32800, Message: "Action canceled"}
				}
			}
			if err := reply(done.id, done.result, failure); err != nil {
				return err
			}
		case incoming := <-messages:
			if incoming.err != nil {
				if errors.Is(incoming.err, io.EOF) {
					return nil
				}
				return incoming.err
			}
			m := incoming.m
			if m.Method == "" { // Sidecars never receive responses/reverse RPC.
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
				if levelVar != nil && p.Settings[SettingLogLevel] != "" {
					levelVar.Set(ParseLogLevel(p.Settings[SettingLogLevel]))
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
				if !initialized || activeID != "" || m.ID == 0 ||
					DecodePayload(m.Params, &p) != nil || p.Validate() != nil {
					return ErrProtocol
				}
				if levelVar != nil && p.Settings[SettingLogLevel] != "" {
					levelVar.Set(ParseLogLevel(p.Settings[SettingLogLevel]))
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
