// Echo Live is an independently built sidecar demonstrating downloadable
// custom native UI via pluginapi.ServeLive. It imports only the public SDK.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/seiortech/mtools-sdk/pluginapi"
)

type echoLive struct {
	draft    string
	history  []string
	selected string
	cardName string
}

func main() {
	identity := pluginapi.InitializeResult{
		Protocol: pluginapi.Protocol,
		PluginID: "org.example.echo-live",
		Version:  "1.0.0",
	}
	app := &echoLive{
		draft:    "Hello from Live UI!",
		history:  []string{"Ready."},
		selected: "item_0",
	}
	runner := pluginapi.RunnerFunc(func(ctx context.Context, p pluginapi.RunParams, emit func(pluginapi.Progress) error) (pluginapi.RunResult, error) {
		_ = emit(pluginapi.Progress{JobID: p.JobID, Done: 1, Total: 1, Unit: "actions", Message: "Saved echo log"})
		return pluginapi.RunResult{
			JobID:   p.JobID,
			Outputs: []string{},
			Summary: "Saved echo log via live action.",
		}, nil
	})
	if err := pluginapi.ServeLive(context.Background(), os.Stdin, os.Stdout, identity, runner, app); err != nil {
		os.Exit(1)
	}
}

func (e *echoLive) Open(_ context.Context, _ pluginapi.UIOpenParams, s *pluginapi.UISession) (pluginapi.UITree, error) {
	s.Logger().Info("echo-live workspace opened", "component", "echo_live")
	e.writeCard(s.AssetsDir(), "card_0.png", len(e.history))
	e.cardName = "card_0.png"
	return e.buildTree(), nil
}

func (e *echoLive) Event(_ context.Context, ev pluginapi.UIEvent, s *pluginapi.UISession) error {
	switch {
	case ev.Node == "input" && ev.Kind == pluginapi.UIEventChange:
		e.draft = ev.Value
		return nil
	case (ev.Node == "echo_btn" && ev.Kind == pluginapi.UIEventClick) ||
		(ev.Node == "input" && ev.Kind == pluginapi.UIEventSubmit):
		msg := strings.TrimSpace(e.draft)
		if msg == "" {
			return nil
		}
		e.history = append(e.history, msg)
		e.selected = fmt.Sprintf("item_%d", len(e.history)-1)
		e.cardName = fmt.Sprintf("card_%d.png", len(e.history))
		e.writeCard(s.AssetsDir(), e.cardName, len(e.history))
		s.Logger().Info("echo message added", "component", "echo_live", "count", len(e.history))
		return s.Render(e.buildTree())
	case ev.Node == "history" && ev.Kind == pluginapi.UIEventSelect:
		e.selected = ev.Value
		return s.Render(e.buildTree())
	}
	return nil
}

func (e *echoLive) writeCard(dir, name string, count int) {
	if dir == "" {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, 240, 80))
	bg := color.RGBA{R: 220, G: 230, B: 213, A: 255}
	accent := color.RGBA{R: 74, G: 93, B: 69, A: 255}
	for y := 0; y < 80; y++ {
		for x := 0; x < 240; x++ {
			c := bg
			if x < 2 || x >= 238 || y < 2 || y >= 78 || (x < 12+count*8 && y >= 32 && y < 48) {
				c = accent
			}
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}

func (e *echoLive) buildTree() pluginapi.UITree {
	items := make([]pluginapi.UINode, 0, len(e.history))
	for i, text := range e.history {
		items = append(items, pluginapi.UINode{
			ID:    fmt.Sprintf("item_%d", i),
			Kind:  pluginapi.UINodeText,
			Props: mustProps(pluginapi.UITextProps{Text: text}),
		})
	}
	return pluginapi.UITree{
		Root: pluginapi.UINode{
			ID:    "root",
			Kind:  pluginapi.UINodeSplit,
			Props: mustProps(pluginapi.UISplitProps{Axis: "h", Size: 260, Min: 180, Max: 420}),
			Children: []pluginapi.UINode{
				{
					ID:   "left",
					Kind: pluginapi.UINodeScroll,
					Children: []pluginapi.UINode{
						{
							ID:       "history",
							Kind:     pluginapi.UINodeList,
							Props:    mustProps(pluginapi.UIListProps{Selected: e.selected}),
							Children: items,
						},
					},
				},
				{
					ID:    "right",
					Kind:  pluginapi.UINodeColumn,
					Props: mustProps(pluginapi.UIContainerProps{Gap: 12, Padding: 16, Surface: "paper", Border: true}),
					Children: []pluginapi.UINode{
						{ID: "title", Kind: pluginapi.UINodeText, Props: mustProps(pluginapi.UITextProps{Text: "Echo Live Workbench", Variant: "title"})},
						{ID: "input", Kind: pluginapi.UINodeTextInput, Props: mustProps(pluginapi.UITextInputProps{Value: e.draft, Placeholder: "Type something..."})},
						{
							ID:    "actions",
							Kind:  pluginapi.UINodeRow,
							Props: mustProps(pluginapi.UIContainerProps{Gap: 8}),
							Children: []pluginapi.UINode{
								{ID: "echo_btn", Kind: pluginapi.UINodeButton, Props: mustProps(pluginapi.UIButtonProps{Label: "Echo", Variant: "primary"})},
								{ID: "save_btn", Kind: pluginapi.UINodeButton, Props: mustProps(pluginapi.UIButtonProps{Label: "Save Log", Variant: "secondary", Action: "save_history"})},
							},
						},
						{ID: "card", Kind: pluginapi.UINodeImage, Props: mustProps(pluginapi.UIImageProps{Src: "asset:" + e.cardName, Width: 240, Height: 80, Alt: "Echo activity card"})},
					},
				},
			},
		},
	}
}

func mustProps(v any) map[string]json.RawMessage {
	raw, _ := json.Marshal(v)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	return m
}
