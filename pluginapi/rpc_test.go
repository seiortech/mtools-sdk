package pluginapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

func TestRPCGoldenFrames(t *testing.T) {
	for _, frame := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocol":"jsonrpc-stdio-v1","plugin_id":"org.example.echo","version":"1.0.0"}}`,
		`{"jsonrpc":"2.0","id":1,"result":{"protocol":"jsonrpc-stdio-v1","plugin_id":"org.example.echo","version":"1.0.0"}}`,
		`{"jsonrpc":"2.0","method":"cancel","params":{"job_id":"run-example"}}`,
		`{"jsonrpc":"2.0","id":2,"error":{"code":-32000,"message":"Action failed"}}`,
	} {
		t.Run(frame, func(t *testing.T) {
			m, err := ReadMessage(bufio.NewReader(strings.NewReader(frame + "\n")))
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := WriteMessage(&out, m); err != nil || out.String() != frame+"\n" {
				t.Fatalf("golden mismatch: %s %v", out.String(), err)
			}
		})
	}
}

func TestRPCRejectsAmbiguousFrames(t *testing.T) {
	for name, frame := range map[string]string{
		"duplicate":     `{"jsonrpc":"2.0","id":1,"id":2,"result":{}}` + "\n",
		"nested":        `{"jsonrpc":"2.0","id":1,"result":{"x":1,"x":2}}` + "\n",
		"escaped":       `{"jsonrpc":"2.0","id":1,"\u0069d":2,"result":{}}` + "\n",
		"case":          `{"JSONRPC":"2.0","id":1,"result":{}}` + "\n",
		"unknown":       `{"jsonrpc":"2.0","id":1,"extra":1,"result":{}}` + "\n",
		"null":          `{"jsonrpc":"2.0","id":1,"result":null}` + "\n",
		"batch":         `[]` + "\n",
		"fractional id": `{"jsonrpc":"2.0","id":1.5,"result":{}}` + "\n",
		"string id":     `{"jsonrpc":"2.0","id":"1","result":{}}` + "\n",
		"zero id":       `{"jsonrpc":"2.0","id":0,"method":"progress","params":{}}` + "\n",
		"both":          `{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-1,"message":"x"}}` + "\n",
		"method result": `{"jsonrpc":"2.0","id":1,"method":"run","params":{},"result":{}}` + "\n",
		"no delimiter":  `{"jsonrpc":"2.0","id":1,"result":{}}`,
		"empty":         "\n",
		"trailing":      `{"jsonrpc":"2.0","id":1,"result":{}} {}` + "\n",
		"large":         strings.Repeat(" ", MaxMessageBytes+1) + "\n",
		"utf8":          "\xff\n",
		"deep":          `{"jsonrpc":"2.0","id":1,"result":` + strings.Repeat("[", 20) + "0" + strings.Repeat("]", 20) + "}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadMessage(bufio.NewReader(strings.NewReader(frame))); !errors.Is(err, ErrProtocol) {
				t.Fatalf("accepted frame: %v", err)
			}
		})
	}
	if _, err := ReadMessage(bufio.NewReader(strings.NewReader(""))); !errors.Is(err, io.EOF) {
		t.Fatalf("empty stream: %v", err)
	}
}

func TestRPCPayloadBounds(t *testing.T) {
	valid := RunParams{JobID: "run-one", Tool: "echo", Action: "echo", Inputs: map[string][]string{}, Settings: map[string]string{}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RunParams){
		func(p *RunParams) { p.JobID = "../escape" },
		func(p *RunParams) { p.Tool = "" },
		func(p *RunParams) { p.Action = "RUN" },
		func(p *RunParams) { p.Inputs = nil },
		func(p *RunParams) { p.Settings = nil },
		func(p *RunParams) { p.Inputs = map[string][]string{"text": nil} },
		func(p *RunParams) { p.Inputs = map[string][]string{"text": {strings.Repeat("x", 8193)}} },
		func(p *RunParams) { p.Inputs = map[string][]string{"text": {"secret\x00"}} },
		func(p *RunParams) {
			p.Inputs = map[string][]string{"text": make([]string, 256)}
			for i := range p.Inputs["text"] {
				p.Inputs["text"][i] = strings.Repeat("x", 8192)
			}
		},
	} {
		p := valid
		mutate(&p)
		if p.Validate() == nil {
			t.Fatalf("accepted %#v", p)
		}
	}
	for _, p := range []Progress{
		{JobID: "stale"}, {JobID: valid.JobID, Done: -1}, {JobID: valid.JobID, Done: math.Inf(1)},
		{JobID: valid.JobID, Done: math.NaN()}, {JobID: valid.JobID, Done: 2, Total: 1},
	} {
		if p.Validate(valid.JobID) == nil {
			t.Fatalf("accepted invalid progress: %#v", p)
		}
	}
	var p RunParams
	if err := DecodePayload([]byte(`{"job_id":"run-one","TOOL":"echo"}`), &p); !errors.Is(err, ErrProtocol) {
		t.Fatalf("case alias: %v", err)
	}
	if err := DecodePayload([]byte(`{}`), nil); !errors.Is(err, ErrProtocol) {
		t.Fatalf("nil destination: %v", err)
	}
	var ptr *RunParams
	if err := DecodePayload([]byte(`{}`), ptr); !errors.Is(err, ErrProtocol) {
		t.Fatalf("nil pointer destination: %v", err)
	}
	failure := &RPCError{Code: -32000, Message: "private credential"}
	if strings.Contains(failure.Error(), "private") {
		t.Fatal("error string leaks remote text")
	}
}

func TestRPCFrameLimitWithoutDelimiter(t *testing.T) {
	// A live pipe must fail at the bound, not wait for a newline/EOF or an
	// additional full bufio buffer. This caught the original ReadSlice deadlock.
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	written := make(chan error, 1)
	go func() {
		_, err := writer.Write(bytes.Repeat([]byte("x"), MaxMessageBytes))
		written <- err
	}()
	if _, err := ReadMessage(bufio.NewReader(reader)); !errors.Is(err, ErrProtocol) {
		t.Fatalf("over-limit live frame: %v", err)
	}
	reader.Close()
	if err := <-written; err != nil {
		t.Fatal(err)
	}
}

func FuzzDecodeRPC(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			return
		}
		m, err := DecodeMessage(data)
		if err != nil {
			return
		}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeMessage(raw); err != nil {
			t.Fatalf("accepted message cannot roundtrip: %v", err)
		}
	})
}
