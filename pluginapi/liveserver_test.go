package pluginapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
)

type fakeLiveUI struct {
	events         []UIEvent
	sharedToolsDir string
}

func (f *fakeLiveUI) Open(ctx context.Context, p UIOpenParams, s *UISession) (UITree, error) {
	f.sharedToolsDir = s.SharedToolsDir()
	return UITree{
		Root: UINode{
			ID:   "root",
			Kind: UINodeColumn,
			Children: []UINode{
				{ID: "msg", Kind: UINodeText, Props: map[string]json.RawMessage{"text": json.RawMessage(`"ready"`)}},
			},
		},
	}, nil
}

func (f *fakeLiveUI) Event(ctx context.Context, ev UIEvent, s *UISession) error {
	f.events = append(f.events, ev)
	return s.Render(UITree{
		Root: UINode{
			ID:   "root",
			Kind: UINodeColumn,
			Children: []UINode{
				{ID: "msg", Kind: UINodeText, Props: map[string]json.RawMessage{"text": json.RawMessage(`"updated"`)}},
			},
		},
	})
}

func TestServeLiveLifecycle(t *testing.T) {
	inRead, inWrite := io.Pipe()
	outRead, outWrite := io.Pipe()

	id := InitializeResult{Protocol: Protocol, PluginID: "org.example.live", Version: "1.0.0"}
	live := &fakeLiveUI{}
	done := make(chan error, 1)
	go func() {
		done <- ServeLive(context.Background(), inRead, outWrite, id, nil, live)
	}()

	send := func(m Message) {
		t.Helper()
		if err := WriteMessage(inWrite, m); err != nil {
			t.Fatal(err)
		}
	}
	reader := bufio.NewReader(outRead)
	recv := func() Message {
		t.Helper()
		m, err := ReadMessage(reader)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	initParams, _ := json.Marshal(id)
	send(Message{JSONRPC: "2.0", ID: 1, Method: MethodInitialize, Params: initParams})
	r1 := recv()
	if r1.ID != 1 || r1.Error != nil {
		t.Fatalf("unexpected init reply: %+v", r1)
	}

	openParams, _ := json.Marshal(UIOpenParams{
		Tool:     "echo",
		Viewport: UIViewport{Width: 800, Height: 600},
		Settings: map[string]string{SettingSharedToolsDir: "/tmp/mtools-shared-tools"},
	})
	send(Message{JSONRPC: "2.0", ID: 2, Method: MethodUIOpen, Params: openParams})
	r2 := recv()
	if r2.ID != 2 || r2.Error != nil {
		t.Fatalf("unexpected open reply: %+v", r2)
	}
	if live.sharedToolsDir != "/tmp/mtools-shared-tools" {
		t.Fatalf("expected session.SharedToolsDir() == /tmp/mtools-shared-tools, got %q", live.sharedToolsDir)
	}
	tree, err := DecodeTree(r2.Result)
	if err != nil || tree.Rev != 1 {
		t.Fatalf("invalid open tree: %+v err=%v", tree, err)
	}

	evParams, _ := json.Marshal(UIEvent{Rev: 1, Node: "btn", Kind: UIEventClick})
	send(Message{JSONRPC: "2.0", Method: MethodUIEvent, Params: evParams})
	n := recv()
	if n.Method != MethodUITree {
		t.Fatalf("expected ui.tree notification, got %+v", n)
	}
	tree2, err := DecodeTree(n.Params)
	if err != nil || tree2.Rev != 2 {
		t.Fatalf("invalid pushed tree: %+v err=%v", tree2, err)
	}

	closeParams, _ := json.Marshal(UICloseParams{Tool: "echo"})
	send(Message{JSONRPC: "2.0", Method: MethodUIClose, Params: closeParams})
	if err := <-done; err != nil {
		t.Fatalf("ServeLive exited with error: %v", err)
	}
}

func TestUISessionRenderConcurrentOrder(t *testing.T) {
	var mu sync.Mutex
	var revs []uint64
	s := &UISession{
		write: func(m Message) error {
			tree, err := DecodeTree(m.Params)
			if err != nil {
				return err
			}
			mu.Lock()
			revs = append(revs, tree.Rev)
			mu.Unlock()
			return nil
		},
	}

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Render(UITree{
				Root: UINode{ID: "root", Kind: UINodeColumn},
			})
			if err != nil {
				t.Errorf("Render failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if len(revs) != 16 {
		t.Fatalf("expected 16 rendered trees, got %d", len(revs))
	}
	for i := 1; i < len(revs); i++ {
		if revs[i] <= revs[i-1] {
			t.Fatalf("out-of-order revisions on wire: %v", revs)
		}
	}
}
