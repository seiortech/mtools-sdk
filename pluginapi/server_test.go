package pluginapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestServeCancellationAndEOF(t *testing.T) {
	host, child := net.Pipe()
	defer host.Close()
	host.SetDeadline(time.Now().Add(5 * time.Second))
	identity := InitializeResult{Protocol: Protocol, PluginID: "org.example.echo", Version: "1.0.0"}
	finished := make(chan error, 1)
	go func() {
		finished <- Serve(t.Context(), child, child, identity, RunnerFunc(func(ctx context.Context, p RunParams, emit func(Progress) error) (RunResult, error) {
			if err := emit(Progress{JobID: p.JobID, Message: "ready"}); err != nil {
				return RunResult{}, err
			}
			<-ctx.Done()
			return RunResult{}, ctx.Err()
		}))
	}()
	send := func(id uint64, method string, p any) {
		t.Helper()
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := WriteMessage(host, Message{JSONRPC: "2.0", ID: id, Method: method, Params: raw}); err != nil {
			t.Fatal(err)
		}
	}
	reader := bufio.NewReader(host)
	send(1, MethodInitialize, identity)
	if m, err := ReadMessage(reader); err != nil || m.ID != 1 {
		t.Fatalf("initialize: %v %v", m, err)
	}
	send(2, MethodRun, RunParams{JobID: "run-test", Tool: "echo", Action: "hold", Inputs: map[string][]string{}, Settings: map[string]string{}})
	if m, err := ReadMessage(reader); err != nil || m.Method != MethodProgress {
		t.Fatalf("progress: %v %v", m, err)
	}
	send(0, MethodCancel, CancelParams{JobID: "run-test"})
	if m, err := ReadMessage(reader); err != nil || m.ID != 2 || m.Error == nil || m.Error.Code != -32800 {
		t.Fatalf("cancel response: %v %v", m, err)
	}
	host.Close()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop at EOF")
	}
}
