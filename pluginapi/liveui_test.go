package pluginapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSurfaceLiveRequiresCapability(t *testing.T) {
	manifest := Manifest{
		Schema:       1,
		ID:           "org.example.live",
		Name:         "Live",
		Version:      "1.0.0",
		Platform:     "darwin-arm64",
		Publisher:    Publisher{Name: "Example", PublicKey: strings.Repeat("A", 43) + "="},
		Capabilities: []Capability{Process},
		Entry:        Entry{Kind: "sidecar", Path: "bin/live", Protocol: Protocol},
		Tools:        []Tool{{ID: "live", Name: "Live", Description: "Live tool", Category: "General"}},
		Files:        map[string]FileInfo{"bin/live": {SHA256: strings.Repeat("a", 64), Size: 10, Executable: true}},
	}
	// Fix public key to valid 32-byte base64
	manifest.Publisher.PublicKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

	raw := []byte(`{
		"schema": 1,
		"tools": [{
			"id": "live",
			"live": {"initial_title": "Loading..."},
			"fields": [],
			"actions": []
		}]
	}`)

	if _, err := DecodeSurfaces(raw, manifest); !errors.Is(err, ErrSurface) {
		t.Fatalf("expected ErrSurface without ui.live capability, got %v", err)
	}

	manifest.Capabilities = append(manifest.Capabilities, UILive)
	doc, err := DecodeSurfaces(raw, manifest)
	if err != nil {
		t.Fatalf("expected valid live surface with ui.live, got %v", err)
	}
	if doc.Tools[0].Live == nil || doc.Tools[0].Live.InitialTitle != "Loading..." {
		t.Fatalf("unexpected Live config: %+v", doc.Tools[0].Live)
	}
}

func TestDecodeTreeValidAndLimits(t *testing.T) {
	valid := []byte(`{
		"rev": 1,
		"root": {
			"id": "root",
			"kind": "split",
			"props": {"axis": "h", "size": 280, "min": 200, "max": 420},
			"children": [
				{
					"id": "left",
					"kind": "scroll",
					"children": [{
						"id": "items",
						"kind": "list",
						"props": {"selected": "row1"},
						"children": [
							{"id": "row1", "kind": "text", "props": {"text": "Hello", "variant": "title"}}
						]
					}]
				},
				{
					"id": "right",
					"kind": "column",
					"props": {"gap": 12, "padding": 16, "surface": "paper", "border": true},
					"children": [
						{"id": "inp", "kind": "text_input", "props": {"value": "hi", "placeholder": "Type..."}},
						{"id": "btn", "kind": "button", "props": {"label": "Run", "variant": "primary", "action": "save"}},
						{"id": "img", "kind": "image", "props": {"src": "asset:card.png", "fit": "contain", "alt": "Card"}},
						{"id": "prog", "kind": "progress", "props": {"done": 1, "total": 2, "label": "Half"}},
						{"id": "emp", "kind": "empty", "props": {"title": "Empty", "subtitle": "Nothing yet"}},
						{"id": "sel", "kind": "select", "props": {"label": "Ink", "value": "orig", "options": [{"value": "orig", "label": "Original"}]}},
						{"id": "tog", "kind": "toggle", "props": {"label": "Remove BG", "checked": true, "style": "chip"}},
						{"id": "tabs", "kind": "tabs", "props": {"active": "vault", "tabs": [{"value": "vault", "label": "Vault"}, {"value": "draw", "label": "Draw"}]}},
						{"id": "fp", "kind": "file_picker", "props": {"label": "Open PDF", "mode": "open", "extensions": ["pdf"]}},
						{"id": "dp", "kind": "dir_picker", "props": {"label": "Output Folder", "value": "/tmp"}},
						{"id": "canv", "kind": "canvas", "props": {"width": 560, "height": 240, "src": "asset:page.png", "interactive": true, "ops": [{"kind": "stroke_path", "token": "ink", "width": 3, "points": [10, 10, 20, 20]}]}, "children": [
							{"id": "drag1", "kind": "draggable", "props": {"x": 50, "y": 60, "width": 150, "height": 60, "bound_w": 560, "bound_h": 240, "selected": true}, "children": [
								{"id": "sig_img", "kind": "image", "props": {"src": "asset:sig1.png", "width": 150, "height": 60}}
							]}
						]}
					]
				}
			]
		}
	}`)
	tree, err := DecodeTree(valid)
	if err != nil {
		t.Fatalf("expected valid tree, got %v", err)
	}
	if tree.Rev != 1 || tree.Root.ID != "root" {
		t.Fatalf("unexpected decoded tree: %+v", tree)
	}

	cases := []struct {
		name string
		raw  string
	}{
		{"zero_rev", `{"rev":0,"root":{"id":"r","kind":"column"}}`},
		{"dup_id", `{"rev":1,"root":{"id":"r","kind":"column","children":[{"id":"r","kind":"text","props":{"text":"a"}}]}}`},
		{"bad_id", `{"rev":1,"root":{"id":"Bad ID!","kind":"column"}}`},
		{"unknown_kind", `{"rev":1,"root":{"id":"r","kind":"webview"}}`},
		{"unknown_prop", `{"rev":1,"root":{"id":"r","kind":"text","props":{"text":"a","color":"#ff0000"}}}`},
		{"text_with_children", `{"rev":1,"root":{"id":"r","kind":"text","props":{"text":"a"},"children":[{"id":"c","kind":"column"}]}}`},
		{"split_one_child", `{"rev":1,"root":{"id":"r","kind":"split","children":[{"id":"a","kind":"column"}]}}`},
		{"image_escape", `{"rev":1,"root":{"id":"r","kind":"image","props":{"src":"asset:../secret.png"}}}`},
		{"image_http", `{"rev":1,"root":{"id":"r","kind":"image","props":{"src":"https://example.com/a.png"}}}`},
		{"progress_overflow", `{"rev":1,"root":{"id":"r","kind":"progress","props":{"done":5,"total":2}}}`},
		{"canvas_bad_token", `{"rev":1,"root":{"id":"r","kind":"canvas","props":{"width":100,"height":100,"ops":[{"kind":"rect","token":"#ff0000","w":10,"h":10}]}}}`},
		{"canvas_odd_stroke", `{"rev":1,"root":{"id":"r","kind":"canvas","props":{"width":100,"height":100,"ops":[{"kind":"stroke_path","points":[1,2,3]}]}}}`},
		{"select_empty_options", `{"rev":1,"root":{"id":"r","kind":"select","props":{"label":"L","options":[]}}}`},
		{"file_picker_bad_mode", `{"rev":1,"root":{"id":"r","kind":"file_picker","props":{"label":"Open","mode":"exec"}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeTree([]byte(tc.raw)); !errors.Is(err, ErrUITree) {
				t.Fatalf("%s: expected ErrUITree, got %v", tc.name, err)
			}
		})
	}
}

func TestTreeDepthAndNodeCountLimits(t *testing.T) {
	// Depth 33 should fail
	deep := UINode{ID: "n0", Kind: UINodeColumn}
	cur := &deep
	for i := 1; i <= MaxUIDepth; i++ {
		cur.Children = []UINode{{ID: fmt.Sprintf("n%d", i), Kind: UINodeColumn}}
		cur = &cur.Children[0]
	}
	if err := (UITree{Rev: 1, Root: deep}).Validate(); !errors.Is(err, ErrUITree) {
		t.Fatalf("expected depth limit error, got %v", err)
	}

	// Node count > 2048 should fail
	root := UINode{ID: "root", Kind: UINodeColumn}
	for i := 0; i < MaxUINodes; i++ {
		root.Children = append(root.Children, UINode{
			ID:    fmt.Sprintf("t%d", i),
			Kind:  UINodeText,
			Props: map[string]json.RawMessage{"text": json.RawMessage(`"x"`)},
		})
	}
	if err := (UITree{Rev: 1, Root: root}).Validate(); !errors.Is(err, ErrUITree) {
		t.Fatalf("expected node count limit error, got %v", err)
	}
}

func FuzzDecodeTree(f *testing.F) {
	f.Add([]byte(`{"rev":1,"root":{"id":"root","kind":"column","children":[{"id":"t","kind":"text","props":{"text":"hi"}}]}}`))
	f.Add([]byte(`{"rev":0,"root":{"id":"x","kind":"unknown"}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if bytes.ContainsRune(data, 0) {
			return
		}
		_, _ = DecodeTree(data)
	})
}

func TestUITreeExtendedNodes(t *testing.T) {
	mustProps := func(v any) map[string]json.RawMessage {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]json.RawMessage
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	tree := UITree{
		Rev: 1,
		Root: UINode{
			ID:   "root",
			Kind: UINodeStack,
			Children: []UINode{
				{ID: "ic", Kind: UINodeIcon, Props: mustProps(UIIconProps{Name: "signature", Size: 20, Tone: "accent"})},
				{ID: "sl", Kind: UINodeSlider, Props: mustProps(UISliderProps{Label: "Threshold", Min: 0.2, Max: 0.98, Step: 0.02, Value: 0.6})},
				{ID: "fp", Kind: UINodeFilePicker, Props: mustProps(UIFilePickerProps{Label: "Add PDFs", Mode: "open", Extensions: []string{"pdf"}, Multiple: true})},
				{ID: "dr", Kind: UINodeDraggable, Props: mustProps(UIDraggableProps{X: 10, Y: 20, Width: 120, Height: 60, BoundW: 600, BoundH: 800, Resizable: true, AspectRatio: 2.0})},
				{ID: "dlg", Kind: UINodeDialog, Props: mustProps(UIDialogProps{Title: "Studio", Open: true, Width: 560, MaxWidth: 680}), Children: []UINode{
					{ID: "btn", Kind: UINodeButton, Props: mustProps(UIButtonProps{Label: "Save", Variant: "primary", Icon: "check"})},
				}},
			},
		},
	}
	if err := tree.Validate(); err != nil {
		t.Fatalf("expected valid extended tree, got %v", err)
	}
	if err := (UIEvent{Rev: 1, Node: "dlg", Kind: UIEventClose}).Validate(); err != nil {
		t.Fatalf("expected valid close event, got %v", err)
	}

	badSlider := UITree{
		Rev: 1,
		Root: UINode{ID: "s", Kind: UINodeSlider, Props: mustProps(UISliderProps{Min: 1, Max: 0, Value: 0.5})},
	}
	if err := badSlider.Validate(); err == nil {
		t.Fatal("expected error for inverted slider min/max")
	}

	leftVal := 12.0
	rich := UITree{
		Rev: 2,
		Root: UINode{
			ID:   "ws",
			Kind: UINodeColumn,
			Props: mustProps(UIContainerProps{
				Fill:        true,
				Shortcuts:   []string{"Escape", "L", "Cmd+L"},
				BorderSides: [4]int{0, 0, 4, 0},
				Shadow:      [2]int{6, 6},
			}),
			Children: []UINode{
				{
					ID:   "toolbar",
					Kind: UINodeRow,
					Props: mustProps(UIContainerProps{
						Absolute: true,
						Left:     &leftVal,
						Justify:  "between",
						Align:    "center",
						Surface:  "paper",
					}),
					Children: []UINode{
						{
							ID:   "swatch_blue",
							Kind: UINodeButton,
							Props: mustProps(UIButtonProps{
								Label:       "Blue Ink",
								Variant:     "swatch",
								SwatchToken: "blue_ink",
								Width:       24,
								Height:      24,
							}),
						},
						{
							ID:   "open_pdf",
							Kind: UINodeButton,
							Props: mustProps(UIButtonProps{
								Label:    "Open",
								PickFile: &UIFilePickerProps{Label: "Open PDF", Mode: "open", Extensions: []string{"pdf"}},
							}),
						},
					},
				},
			},
		},
	}
	if err := rich.Validate(); err != nil {
		t.Fatalf("expected valid rich tree, got %v", err)
	}
	for _, ev := range []UIEvent{
		{Rev: 2, Node: "p1", Kind: UIEventDrop, Value: "120.5,88.0|sig_1"},
		{Rev: 2, Node: "vp", Kind: UIEventScroll, Value: "320,800,600"},
		{Rev: 2, Node: "vp", Kind: UIEventZoom, Value: "1.25"},
		{Rev: 2, Node: "ws", Kind: UIEventShortcut, Value: "Cmd+L"},
	} {
		if err := ev.Validate(); err != nil {
			t.Fatalf("expected valid event %+v, got %v", ev, err)
		}
	}

	validHostBtns := []UIButtonProps{
		{Label: "Settings", HostAction: "open_app_settings"},
		{Label: "Download", HostAction: "run_app_setting_action", HostSettingKey: "pdfium_wasm_path"},
	}
	for _, bp := range validHostBtns {
		if !validButtonProps(bp) {
			t.Fatalf("expected valid button props: %+v", bp)
		}
	}
	invalidHostBtns := []UIButtonProps{
		{Label: "Bad", HostAction: "unknown_action"},
		{Label: "MissingKey", HostAction: "run_app_setting_action"},
		{Label: "ExtraKey", HostAction: "open_app_settings", HostSettingKey: "pdfium_wasm_path"},
		{Label: "OrphanKey", HostSettingKey: "pdfium_wasm_path"},
	}
	for _, bp := range invalidHostBtns {
		if validButtonProps(bp) {
			t.Fatalf("expected invalid button props: %+v", bp)
		}
	}
}

func TestVideoSelectStyleAndTextInputWidthValidation(t *testing.T) {
	toProps := func(v any) map[string]json.RawMessage {
		raw, _ := json.Marshal(v)
		var out map[string]json.RawMessage
		_ = json.Unmarshal(raw, &out)
		return out
	}
	vol := 0.8
	valid := UITree{
		Rev: 1,
		Root: UINode{
			ID:   "root",
			Kind: UINodeColumn,
			Children: []UINode{
				{
					ID:   "vid",
					Kind: UINodeVideo,
					Props: toProps(UIVideoProps{
						Src:        "file:/tmp/demo.mp4",
						Fit:        "contain",
						Surface:    "stage",
						Fill:       true,
						Borderless: true,
						Rate:       1.5,
						Volume:     &vol,
						Poster:     "asset:poster.jpg",
					}),
				},
				{
					ID:   "loading_modal",
					Kind: UINodeDialog,
					Props: toProps(UIDialogProps{
						Title:    "Loading media…",
						Open:     true,
						Blocking: true,
						Width:    420,
					}),
				},
				{
					ID:   "sel",
					Kind: UINodeSelect,
					Props: toProps(UISelectProps{
						Label:   "Container",
						Value:   "mp4",
						Style:   "dropdown",
						Options: []SurfaceOption{{Label: "MP4", Value: "mp4"}},
					}),
				},
				{
					ID:   "inp",
					Kind: UINodeTextInput,
					Props: toProps(UITextInputProps{
						Value: "00:10",
						Width: 78,
					}),
				},
				{
					ID:   "compact_tabs",
					Kind: UINodeTabs,
					Props: toProps(UITabsProps{
						Active:  "a",
						Compact: true,
						Tabs:    []SurfaceOption{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}},
					}),
				},
			},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid video/select/input tree, got %v", err)
	}
	if err := (UIEvent{Rev: 1, Node: "vid", Kind: UIEventMedia, Value: "pos=1.200|dur=10.000|playing=1|ended=0|ready=1|err="}).Validate(); err != nil {
		t.Fatalf("expected valid UIEventMedia, got %v", err)
	}

	invalidCases := []struct {
		name string
		raw  string
	}{
		{"video_http_src", `{"rev":1,"root":{"id":"v","kind":"video","props":{"src":"https://example.com/a.mp4"}}}`},
		{"video_bad_rate", `{"rev":1,"root":{"id":"v","kind":"video","props":{"src":"asset:a.mp4","rate":10}}}`},
		{"video_bad_volume", `{"rev":1,"root":{"id":"v","kind":"video","props":{"src":"asset:a.mp4","volume":1.5}}}`},
		{"video_bad_surface", `{"rev":1,"root":{"id":"v","kind":"video","props":{"src":"asset:a.mp4","surface":"ink"}}}`},
		{"video_children", `{"rev":1,"root":{"id":"v","kind":"video","props":{"src":"asset:a.mp4"},"children":[{"id":"c","kind":"column"}]}}`},
		{"select_bad_style", `{"rev":1,"root":{"id":"s","kind":"select","props":{"style":"pills","options":[{"label":"A","value":"a"}]}}}`},
		{"input_bad_width", `{"rev":1,"root":{"id":"i","kind":"text_input","props":{"width":9999}}}`},
	}
	for _, tc := range invalidCases {
		if _, err := DecodeTree([]byte(tc.raw)); err == nil {
			t.Fatalf("case %s: expected ErrUITree, got nil", tc.name)
		}
	}
}
