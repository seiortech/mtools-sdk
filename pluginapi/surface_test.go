package pluginapi

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func echoSurface(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile("../examples/plugins/echo/surfaces.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSurfaceExampleAndStrictDecoding(t *testing.T) {
	data := echoSurface(t)
	d, err := DecodeSurfaces(data, validManifest())
	if err != nil || d.Tools[0].Fields[0].Default == "" {
		t.Fatalf("example: %v", err)
	}
	for name, bad := range map[string][]byte{
		"unknown":   []byte(strings.Replace(string(data), `"schema":`, `"unknown":1,"schema":`, 1)),
		"duplicate": []byte(strings.Replace(string(data), `"schema":`, `"schema":1,"schema":`, 1)),
		"case":      []byte(strings.Replace(string(data), `"label":`, `"Label":`, 1)),
		"null":      []byte(`null`),
		"trailing":  append(slices.Clone(data), []byte(`{}`)...),
		"oversize":  []byte(strings.Repeat(" ", MaxSurfaceBytes+1)),
		"encoding":  {0xff},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSurfaces(bad, validManifest()); !errors.Is(err, ErrSurface) {
				t.Fatalf("accepted: %v", err)
			}
		})
	}
}

func TestSurfaceValidation(t *testing.T) {
	for name, mutate := range map[string]func(*SurfaceDocument){
		"schema":          func(d *SurfaceDocument) { d.Schema++ },
		"unknown tool":    func(d *SurfaceDocument) { d.Tools[0].ID = "other" },
		"duplicate tool":  func(d *SurfaceDocument) { d.Tools = append(d.Tools, d.Tools[0]) },
		"no action":       func(d *SurfaceDocument) { d.Tools[0].Actions = nil },
		"unknown input":   func(d *SurfaceDocument) { d.Tools[0].Actions[0].Inputs = []string{"other"} },
		"duplicate input": func(d *SurfaceDocument) { d.Tools[0].Actions[0].Inputs = []string{"text", "text"} },
		"duplicate field": func(d *SurfaceDocument) { d.Tools[0].Fields = append(d.Tools[0].Fields, d.Tools[0].Fields[0]) },
		"control label":   func(d *SurfaceDocument) { d.Tools[0].Fields[0].Label = "unsafe\nlabel" },
		"kind":            func(d *SurfaceDocument) { d.Tools[0].Fields[0].Kind = "script" },
		"toggle":          func(d *SurfaceDocument) { d.Tools[0].Fields[0].Kind = "toggle" },
		"file grant":      func(d *SurfaceDocument) { d.Tools[0].Fields[0].Kind = "files"; d.Tools[0].Fields[0].Default = "" },
		"directory grant": func(d *SurfaceDocument) { d.Tools[0].Fields[0].Kind = "directory"; d.Tools[0].Fields[0].Default = "" },
		"choice":          func(d *SurfaceDocument) { d.Tools[0].Fields[0].Kind = "choice" },
		"field bound":     func(d *SurfaceDocument) { d.Tools[0].Fields = make([]SurfaceField, 65) },
		"action bound":    func(d *SurfaceDocument) { d.Tools[0].Actions = make([]SurfaceAction, 17) },
	} {
		t.Run(name, func(t *testing.T) {
			d, err := DecodeSurfaces(echoSurface(t), validManifest())
			if err != nil {
				t.Fatal(err)
			}
			mutate(&d)
			if !errors.Is(d.Validate(validManifest()), ErrSurface) {
				t.Fatal("accepted invalid surface")
			}
		})
	}
}

func TestSurfaceInputs(t *testing.T) {
	m := validManifest()
	m.Capabilities = append(m.Capabilities, FilesRead, FilesWrite)
	d := SurfaceDocument{Schema: 1, Tools: []ToolSurface{{
		ID: "echo",
		Fields: []SurfaceField{
			{ID: "text", Kind: "text", Label: "Text", Required: true},
			{ID: "files", Kind: "files", Label: "Files", Required: true},
			{ID: "directory", Kind: "directory", Label: "Directory", Required: true},
			{ID: "choice", Kind: "choice", Label: "Choice", Options: []SurfaceOption{{Label: "One", Value: "one"}}},
			{ID: "toggle", Kind: "toggle", Label: "Toggle"},
		},
		Actions: []SurfaceAction{
			{ID: "text", Label: "Text", Inputs: []string{"text"}},
			{ID: "all", Label: "All", Inputs: []string{"text", "files", "directory", "choice", "toggle"}},
		},
	}}}
	if err := d.Validate(m); err != nil {
		t.Fatal(err)
	}
	tool := d.Tools[0]
	values := map[string][]string{"text": {"hello"}, "files": {filepath.Join(t.TempDir(), "input")}, "directory": {t.TempDir()}, "choice": {"one"}, "toggle": {"false"}, "unknown": {"secret"}}
	inputs, err := tool.Inputs("text", values)
	if err != nil || len(inputs) != 1 {
		t.Fatalf("action scope: %v", err)
	}
	inputs["text"][0] = "mutated"
	if values["text"][0] != "hello" {
		t.Fatal("input snapshot aliased")
	}
	if _, err := tool.Inputs("all", values); err != nil {
		t.Fatal(err)
	}
	for field, bad := range map[string][]string{
		"text": {" \t"}, "files": {"relative"}, "directory": {}, "choice": {"unknown"}, "toggle": {"yes"},
	} {
		copy := make(map[string][]string)
		for k, v := range values {
			copy[k] = v
		}
		copy[field] = bad
		_, err := tool.Inputs("all", copy)
		var input *InputError
		if !errors.As(err, &input) || input.Field != field {
			t.Fatalf("%s: %v", field, err)
		}
	}
	raw, _ := json.Marshal(d)
	if _, err := DecodeSurfaces(raw, m); err != nil {
		t.Fatal(err)
	}
}

func FuzzDecodeSurfaces(f *testing.F) {
	f.Add(echoSurface(f))
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := DecodeSurfaces(data, validManifest())
		if err == nil && d.Validate(validManifest()) != nil {
			t.Fatal("invalid decoded surface")
		}
	})
}
