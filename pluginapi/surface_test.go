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

func TestSurfaceSectionsChecklistChipsAndConditions(t *testing.T) {
	m := validManifest()
	d := SurfaceDocument{
		Schema: 1,
		Tools: []ToolSurface{{
			ID: "echo",
			Fields: []SurfaceField{
				{ID: "url", Kind: "text", Label: "URL", Grow: true, Required: true},
				{ID: "mode", Kind: "select", Label: "Mode", Default: "a", Options: []SurfaceOption{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}},
				{ID: "cats", Kind: "chips", Label: "Categories", Options: []SurfaceOption{{Label: "One", Value: "one"}, {Label: "Two", Value: "two"}}, VisibleWhen: &SurfaceCondition{Field: "mode", Equals: "b"}},
				{ID: "items", Kind: "checklist", Label: "Items", Dynamic: true},
			},
			Actions: []SurfaceAction{
				{ID: "preview", Label: "Preview", Kind: "inspect", Inputs: []string{"url"}},
				{ID: "run", Label: "Run", Kind: "run", Inputs: []string{"url", "mode", "cats", "items"}},
			},
			Sections: []SurfaceSection{
				{ID: "hero", Layout: "primary", Fields: []string{"url"}, Actions: []string{"preview", "run"}},
				{ID: "opts", Layout: "tabs", Groups: []SurfaceSection{
					{ID: "tab_a", Label: "General", Layout: "wrap", Fields: []string{"mode", "cats"}},
					{ID: "tab_b", Label: "Grid", Layout: "grid", Columns: 2, Fields: []string{"items"}},
				}},
				{ID: "adv", Label: "Advanced", Layout: "collapsible", DefaultOpen: false, Groups: []SurfaceSection{
					{ID: "adv_row", Layout: "horizontal", Fields: []string{"mode"}},
					{ID: "adv_col", Layout: "vertical", Fields: []string{"cats"}},
				}},
			},
		}},
	}
	if err := d.Validate(m); err != nil {
		t.Fatalf("valid extended surface rejected: %v", err)
	}
	raw, _ := json.Marshal(d)
	if _, err := DecodeSurfaces(raw, m); err != nil {
		t.Fatalf("DecodeSurfaces rejected extended surface: %v", err)
	}

	inputs, err := d.Tools[0].Inputs("run", map[string][]string{
		"url":   {"https://example.com"},
		"mode":  {"b"},
		"cats":  {"one", "two"},
		"items": {"v1", "v2"},
	})
	if err != nil || len(inputs["cats"]) != 2 || len(inputs["items"]) != 2 {
		t.Fatalf("Inputs failed: %v %+v", err, inputs)
	}

	// Negative validation checks
	for name, mutate := range map[string]func(*SurfaceDocument){
		"bad layout":       func(doc *SurfaceDocument) { doc.Tools[0].Sections[0].Layout = "carousel" },
		"bad grid cols":    func(doc *SurfaceDocument) { doc.Tools[0].Sections[1].Groups[1].Columns = 5 },
		"cols on non-grid": func(doc *SurfaceDocument) { doc.Tools[0].Sections[0].Columns = 2 },
		"bad action kind":  func(doc *SurfaceDocument) { doc.Tools[0].Actions[0].Kind = "eval" },
		"self visible":     func(doc *SurfaceDocument) { doc.Tools[0].Fields[2].VisibleWhen.Field = "cats" },
		"unknown visible":  func(doc *SurfaceDocument) { doc.Tools[0].Fields[2].VisibleWhen.Field = "missing" },
		"bad history width": func(doc *SurfaceDocument) {
			doc.Tools[0].History = &SurfaceHistory{DefaultWidth: 100}
		},
		"bad history button action": func(doc *SurfaceDocument) {
			doc.Tools[0].History = &SurfaceHistory{
				Buttons: []SurfaceHistoryButton{{ID: "btn1", Label: "Run", Action: "nonexistent"}},
			}
		},
		"bad history button url": func(doc *SurfaceDocument) {
			doc.Tools[0].History = &SurfaceHistory{
				Buttons: []SurfaceHistoryButton{{ID: "btn1", Label: "Open", URL: "http://insecure.local"}},
			}
		},
		"depth overflow": func(doc *SurfaceDocument) {
			doc.Tools[0].Sections[1].Groups[0].Groups = []SurfaceSection{{
				ID: "d2", Layout: "vertical", Groups: []SurfaceSection{{
					ID: "d3", Layout: "vertical", Groups: []SurfaceSection{{
						ID: "d4", Layout: "vertical", Groups: []SurfaceSection{{
							ID: "d5", Layout: "vertical", Groups: []SurfaceSection{{
								ID: "d6", Layout: "vertical",
							}},
						}},
					}},
				}},
			}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var cloned SurfaceDocument
			_ = json.Unmarshal(raw, &cloned)
			mutate(&cloned)
			if !errors.Is(cloned.Validate(m), ErrSurface) {
				t.Fatalf("expected ErrSurface for %s", name)
			}
		})
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
