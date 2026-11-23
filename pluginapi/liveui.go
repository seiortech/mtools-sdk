package pluginapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	MethodUIOpen  = "ui.open"
	MethodUITree  = "ui.tree"
	MethodUIEvent = "ui.event"
	MethodUIClose = "ui.close"

	MaxUINodes      = 2048
	MaxUIDepth      = 32
	MaxUITextBytes       = 8192
	MaxUIAssetBytes      = 32 << 20
	MaxUIVideoAssetBytes = 256 << 20
	MaxUICanvasOps       = 4096
)

const (
	UINodeColumn     = "column"
	UINodeRow        = "row"
	UINodeScroll     = "scroll"
	UINodeText       = "text"
	UINodeButton     = "button"
	UINodeTextInput  = "text_input"
	UINodeImage      = "image"
	UINodeVideo      = "video"
	UINodeSplit      = "split"
	UINodeList       = "list"
	UINodeProgress   = "progress"
	UINodeEmpty      = "empty"
	UINodeSelect     = "select"
	UINodeToggle     = "toggle"
	UINodeTabs       = "tabs"
	UINodeFilePicker = "file_picker"
	UINodeDirPicker  = "dir_picker"
	UINodeDraggable  = "draggable"
	UINodeCanvas     = "canvas"
	UINodeStack      = "stack"
	UINodeSlider     = "slider"
	UINodeDialog     = "dialog"
	UINodeIcon       = "icon"
)

const (
	UIEventClick       = "click"
	UIEventDoubleClick = "double_click"
	UIEventChange      = "change"
	UIEventSubmit      = "submit"
	UIEventSelect      = "select"
	UIEventToggle      = "toggle"
	UIEventResize      = "resize"
	UIEventDragEnd     = "drag_end"
	UIEventDrop        = "drop"
	UIEventStroke      = "stroke"
	UIEventView        = "viewport"
	UIEventScroll      = "scroll"
	UIEventZoom        = "zoom"
	UIEventShortcut    = "shortcut"
	UIEventMedia       = "media"
	UIEventClose       = "close"
)

var ErrUITree = errors.New("invalid plugin live UI tree")

var uiNodeIDPattern = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)

type UIViewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type UIOpenParams struct {
	Tool     ToolID            `json:"tool"`
	Viewport UIViewport        `json:"viewport"`
	Lang     string            `json:"lang,omitempty"`
	Theme    string            `json:"theme,omitempty"` // "light" or "dark"
	Settings map[string]string `json:"settings,omitempty"`
}

// SharedTools returns a SharedToolsProvider backed by p.Settings and
// $MTOOLS_SHARED_TOOLS_DIR.
func (p UIOpenParams) SharedTools() SharedToolsProvider {
	return NewSharedTools(p.Settings)
}

// SharedToolsDir returns the effective default shared tools directory.
func (p UIOpenParams) SharedToolsDir() string {
	return p.SharedTools().SharedToolsDir()
}

func (p UIOpenParams) Validate() error {
	if !localIDPattern.MatchString(string(p.Tool)) {
		return ErrProtocol
	}
	if p.Viewport.Width < 0 || p.Viewport.Height < 0 || p.Viewport.Width > 16384 || p.Viewport.Height > 16384 {
		return ErrProtocol
	}
	if p.Theme != "" && p.Theme != "light" && p.Theme != "dark" {
		return ErrProtocol
	}
	if !textOK(p.Lang, 32, false) || len(p.Settings) > 64 {
		return ErrProtocol
	}
	for k, v := range p.Settings {
		if !localIDPattern.MatchString(k) || !utf8.ValidString(v) || len(v) > MaxUITextBytes ||
			bytes.IndexByte([]byte(v), 0) >= 0 {
			return ErrProtocol
		}
	}
	return nil
}

type UIEvent struct {
	Rev   uint64 `json:"rev"`
	Node  string `json:"node"`
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

func (e UIEvent) Validate() error {
	if e.Node != "" && !uiNodeIDPattern.MatchString(e.Node) {
		return ErrProtocol
	}
	switch e.Kind {
	case UIEventClick, UIEventDoubleClick, UIEventChange, UIEventSubmit, UIEventSelect, UIEventToggle,
		UIEventResize, UIEventDragEnd, UIEventDrop, UIEventStroke, UIEventView, UIEventScroll, UIEventZoom,
		UIEventShortcut, UIEventMedia, UIEventClose:
	default:
		return ErrProtocol
	}
	if !utf8.ValidString(e.Value) || len(e.Value) > MaxUITextBytes || bytes.IndexByte([]byte(e.Value), 0) >= 0 {
		return ErrProtocol
	}
	return nil
}

type UICloseParams struct {
	Tool ToolID `json:"tool"`
}

func (p UICloseParams) Validate() error {
	if !localIDPattern.MatchString(string(p.Tool)) {
		return ErrProtocol
	}
	return nil
}

type UITree struct {
	Rev  uint64 `json:"rev"`
	Root UINode `json:"root"`
}

type UINode struct {
	ID       string                     `json:"id"`
	Kind     string                     `json:"kind"`
	Props    map[string]json.RawMessage `json:"props,omitempty"`
	Children []UINode                   `json:"children,omitempty"`
}

type UIScrollTarget struct {
	Y      float64 `json:"y"`
	Seq    uint64  `json:"seq"`
	Smooth bool    `json:"smooth,omitempty"`
}

type UIContainerProps struct {
	Gap           int             `json:"gap,omitempty"`
	Padding       int             `json:"padding,omitempty"`
	PaddingV      int             `json:"padding_v,omitempty"`
	PaddingH      int             `json:"padding_h,omitempty"`
	Width         int             `json:"width,omitempty"`
	Height        int             `json:"height,omitempty"`
	MinWidth      int             `json:"min_width,omitempty"`
	MinHeight     int             `json:"min_height,omitempty"`
	MaxWidth      int             `json:"max_width,omitempty"`
	MaxHeight     int             `json:"max_height,omitempty"`
	Wrap          bool            `json:"wrap,omitempty"`
	Grow          bool            `json:"grow,omitempty"`
	Fill          bool            `json:"fill,omitempty"`
	FillHeight    bool            `json:"fill_height,omitempty"`
	NoFillWidth   bool            `json:"no_fill_width,omitempty"`
	Absolute      bool            `json:"absolute,omitempty"`
	Left          *float64        `json:"left,omitempty"`
	Top           *float64        `json:"top,omitempty"`
	Right         *float64        `json:"right,omitempty"`
	Bottom        *float64        `json:"bottom,omitempty"`
	Justify       string          `json:"justify,omitempty"` // "", "start", "center", "end", "between"
	Align         string          `json:"align,omitempty"`   // "", "start", "center", "end", "stretch"
	Surface       string          `json:"surface,omitempty"` // "", "ground", "paper", "field_support", "accent", "danger", "success"
	Border        bool            `json:"border,omitempty"`
	BorderSize    int             `json:"border_size,omitempty"`
	BorderSides   [4]int          `json:"border_sides,omitempty"` // top, right, bottom, left
	BorderToken   string          `json:"border_token,omitempty"` // "", "ink", "quiet", "accent", "danger", "success"
	Shadow        [2]int          `json:"shadow,omitempty"`
	Circle        bool            `json:"circle,omitempty"`
	Radius        int             `json:"radius,omitempty"`
	ScrollBoth    bool            `json:"scroll_both,omitempty"`
	TrackScroll   bool            `json:"track_scroll,omitempty"`
	ScrollTarget  *UIScrollTarget `json:"scroll_target,omitempty"`
	DropTarget    bool            `json:"drop_target,omitempty"`
	DropHighlight bool            `json:"drop_highlight,omitempty"`
	Clickable     bool            `json:"clickable,omitempty"`
	Shortcuts     []string        `json:"shortcuts,omitempty"`
	Tooltip       string          `json:"tooltip,omitempty"`
}

type UITextProps struct {
	Text          string  `json:"text"`
	Variant       string  `json:"variant,omitempty"` // "", "title", "heading", "body", "muted", "accent", "danger", "success", "on_field"
	FontSize      int     `json:"font_size,omitempty"`
	FontWeight    int     `json:"font_weight,omitempty"`
	Align         string  `json:"align,omitempty"` // "", "left", "center", "right"
	SingleLine    bool    `json:"single_line,omitempty"`
	TabularNums   bool    `json:"tabular_nums,omitempty"`
	PaddingV      int     `json:"padding_v,omitempty"`
	PaddingH      int     `json:"padding_h,omitempty"`
	Tooltip       string  `json:"tooltip,omitempty"`
	Grow          bool    `json:"grow,omitempty"`
	BindVideoTime string  `json:"bind_video_time,omitempty"` // optional video node ID for 0-IPC live timecode
	BindDuration  float64 `json:"bind_duration,omitempty"`   // fallback duration in seconds for live timecode
}

type UIButtonProps struct {
	Label       string             `json:"label"`
	Variant     string             `json:"variant,omitempty"` // "", "primary", "secondary", "danger", "ghost", "swatch", "success"
	Action      string             `json:"action,omitempty"`  // optional declared run action ID
	Icon        string             `json:"icon,omitempty"`    // optional registered icon name
	IconOnly    bool               `json:"icon_only,omitempty"`
	IconSize    int                `json:"icon_size,omitempty"`
	Width       int                `json:"width,omitempty"`
	Height      int                `json:"height,omitempty"`
	PaddingX    int                `json:"padding_x,omitempty"`
	PaddingY    int                `json:"padding_y,omitempty"`
	SwatchToken string             `json:"swatch_token,omitempty"` // "", "ink", "blue_ink", "danger", "accent", "paper"
	BorderToken string             `json:"border_token,omitempty"`
	BorderSize  int                `json:"border_size,omitempty"`
	Selected    bool               `json:"selected,omitempty"`
	FillWidth   bool               `json:"fill_width,omitempty"`
	FillHeight  bool               `json:"fill_height,omitempty"`
	Grow        bool               `json:"grow,omitempty"`
	Tooltip     string             `json:"tooltip,omitempty"`
	PickFile    *UIFilePickerProps `json:"pick_file,omitempty"`
	ShellAction    string             `json:"shell_action,omitempty"` // "", "reveal", "open"
	ShellPath      string             `json:"shell_path,omitempty"`
	HostAction     string             `json:"host_action,omitempty"`      // "", "open_app_settings", "run_app_setting_action"
	HostSettingKey string             `json:"host_setting_key,omitempty"` // e.g. "pdfium_wasm_path"
	DragPayload    string             `json:"drag_payload,omitempty"`
	DragImage   string             `json:"drag_image,omitempty"`
	DragW       int                `json:"drag_w,omitempty"`
	DragH       int                `json:"drag_h,omitempty"`
	Disabled    bool               `json:"disabled,omitempty"`
}

type UITextInputProps struct {
	Value       string `json:"value,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Width       int    `json:"width,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
	Grow        bool   `json:"grow,omitempty"`
}

type UIImageProps struct {
	Src       string `json:"src"`           // "asset:<relative-path>"
	Fit       string `json:"fit,omitempty"` // "", "contain", "cover"
	Alt       string `json:"alt,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	MaxWidth  int    `json:"max_width,omitempty"`
	MaxHeight int    `json:"max_height,omitempty"`
	Grow      bool   `json:"grow,omitempty"`
}

type UIVideoProps struct {
	Src        string   `json:"src"`                  // "file:<authorized-path>" or "asset:<name>"
	Fit        string   `json:"fit,omitempty"`        // "", "contain", "cover"
	Width      int      `json:"width,omitempty"`
	Height     int      `json:"height,omitempty"`
	MaxWidth   int      `json:"max_width,omitempty"`
	MaxHeight  int      `json:"max_height,omitempty"`
	Grow       bool     `json:"grow,omitempty"`
	Fill       bool     `json:"fill,omitempty"`
	Borderless bool     `json:"borderless,omitempty"`
	Playing    bool     `json:"playing,omitempty"`
	PlaySeq    uint64   `json:"play_seq,omitempty"`
	SeekSec    float64  `json:"seek_sec,omitempty"`
	SeekSeq    uint64   `json:"seek_seq,omitempty"`
	Rate       float64  `json:"rate,omitempty"`       // 0 (default 1.0) or 0.25..4.0
	Volume     *float64 `json:"volume,omitempty"`     // nil (default 1.0) or 0.0..1.0
	Muted      bool     `json:"muted,omitempty"`
	Loop       bool     `json:"loop,omitempty"`
	LoopStart  float64  `json:"loop_start,omitempty"`
	LoopEnd    float64  `json:"loop_end,omitempty"`
	ClampStart float64  `json:"clamp_start,omitempty"` // optional playback lower bound in seconds
	ClampEnd   float64  `json:"clamp_end,omitempty"`   // optional playback upper bound in seconds (active when ClampEnd > ClampStart)
	Poster     string   `json:"poster,omitempty"`      // optional "asset:<name>"
	Surface    string   `json:"surface,omitempty"`     // "", "stage", "ground", "paper"
}

type UISplitProps struct {
	Axis               string  `json:"axis,omitempty"` // "", "h", "v"
	Size               float32 `json:"size,omitempty"`
	Min                float32 `json:"min,omitempty"`
	Max                float32 `json:"max,omitempty"`
	DefaultSize        float32 `json:"default_size,omitempty"`
	Collapsible        bool    `json:"collapsible,omitempty"`
	Collapsed          bool    `json:"collapsed,omitempty"`
	ResetOnDoubleClick bool    `json:"reset_on_double_click,omitempty"`
}

type UIListProps struct {
	Selected string `json:"selected,omitempty"`
	Height   int    `json:"height,omitempty"`
	Grow     bool   `json:"grow,omitempty"`
}

type UIProgressProps struct {
	Done  float64 `json:"done"`
	Total float64 `json:"total"`
	Label string  `json:"label,omitempty"`
}

type UIEmptyProps struct {
	Title       string             `json:"title"`
	Subtitle    string             `json:"subtitle,omitempty"`
	ActionLabel string             `json:"action_label,omitempty"`
	Icon        string             `json:"icon,omitempty"`
	IconSize    int                `json:"icon_size,omitempty"`
	Width       int                `json:"width,omitempty"`
	BorderWidth int                `json:"border_width,omitempty"`
	Shadow      [2]int             `json:"shadow,omitempty"`
	PickFile    *UIFilePickerProps `json:"pick_file,omitempty"`
}

type UISelectProps struct {
	Label    string          `json:"label,omitempty"`
	Value    string          `json:"value,omitempty"`
	Options  []SurfaceOption `json:"options"`
	Style    string          `json:"style,omitempty"` // "", "chips", "dropdown"
	Disabled bool            `json:"disabled,omitempty"`
}

type UIToggleProps struct {
	Label    string `json:"label"`
	Checked  bool   `json:"checked,omitempty"`
	Style    string `json:"style,omitempty"` // "", "chip"
	Disabled bool   `json:"disabled,omitempty"`
}

type UITabsProps struct {
	Active  string          `json:"active"`
	Tabs    []SurfaceOption `json:"tabs"`
	Compact bool            `json:"compact,omitempty"`
}

type UIFilePickerProps struct {
	Label      string   `json:"label"`
	Value      string   `json:"value,omitempty"`
	Mode       string   `json:"mode,omitempty"` // "", "open", "save"
	Extensions []string `json:"extensions,omitempty"`
	Multiple   bool     `json:"multiple,omitempty"`
	Disabled   bool     `json:"disabled,omitempty"`
}

type UIDirPickerProps struct {
	Label    string `json:"label"`
	Value    string `json:"value,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type UIDraggableProps struct {
	X             float64 `json:"x"`
	Y             float64 `json:"y"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	BoundW        int     `json:"bound_w,omitempty"`
	BoundH        int     `json:"bound_h,omitempty"`
	Selected      bool    `json:"selected,omitempty"`
	Resizable     bool    `json:"resizable,omitempty"`
	Handles       string  `json:"handles,omitempty"` // "", "se", "all", "none"
	KeepAspect    bool    `json:"keep_aspect,omitempty"`
	AspectRatio   float64 `json:"aspect_ratio,omitempty"`
	ShowLockBadge bool    `json:"show_lock_badge,omitempty"`
	Disabled      bool    `json:"disabled,omitempty"`
}

type UISliderProps struct {
	Label       string  `json:"label,omitempty"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Step        float64 `json:"step,omitempty"`
	Value       float64 `json:"value"`
	Width       int     `json:"width,omitempty"`
	ValueScale  float64 `json:"value_scale,omitempty"`
	ValueSuffix string  `json:"value_suffix,omitempty"`
	Disabled    bool    `json:"disabled,omitempty"`
}

type UIDialogProps struct {
	Title       string `json:"title,omitempty"`
	Subtitle    string `json:"subtitle,omitempty"`
	Open        bool   `json:"open"`
	Blocking    bool   `json:"blocking,omitempty"`
	Width       int    `json:"width,omitempty"`
	MaxWidth    int    `json:"max_width,omitempty"`
	BorderWidth int    `json:"border_width,omitempty"`
	Shadow      [2]int `json:"shadow,omitempty"`
	Padding     int    `json:"padding,omitempty"`
}

type UIIconProps struct {
	Name string `json:"name"`
	Size int    `json:"size,omitempty"`
	Tone string `json:"tone,omitempty"` // "", "ink", "muted", "paper", "accent", "danger"
}

type UICanvasOp struct {
	Kind   string    `json:"kind"`            // "rect", "line", "stroke_path", "image", "text"
	Token  string    `json:"token,omitempty"` // "", "ink", "blue_ink", "muted", "paper", "ground", "accent", "field_support", "warning", "danger", "scrim"
	X      float64   `json:"x,omitempty"`
	Y      float64   `json:"y,omitempty"`
	W      float64   `json:"w,omitempty"`
	H      float64   `json:"h,omitempty"`
	Width  float64   `json:"width,omitempty"`
	Points []float64 `json:"points,omitempty"`
	Src    string    `json:"src,omitempty"`
	Fit    string    `json:"fit,omitempty"` // for Kind=="image": "", "contain", "cover", "fill"
	Text   string    `json:"text,omitempty"`
}

type UICanvasProps struct {
	Width         int          `json:"width"`
	Height        int          `json:"height"`
	Src           string       `json:"src,omitempty"` // optional background "asset:<name>"
	Interactive   bool         `json:"interactive,omitempty"`
	BrushToken    string       `json:"brush_token,omitempty"` // "", "ink", "blue_ink", "danger", "accent", "paper"
	BrushWidth    float64      `json:"brush_width,omitempty"`
	Eraser        bool         `json:"eraser,omitempty"`
	Surface       string       `json:"surface,omitempty"` // "", "paper", "ground"
	BorderWidth   int          `json:"border_width,omitempty"`
	BorderToken   string       `json:"border_token,omitempty"`
	Shadow        [2]int       `json:"shadow,omitempty"`
	DropTarget    bool         `json:"drop_target,omitempty"`
	DropHighlight bool         `json:"drop_highlight,omitempty"`
	Clickable     bool         `json:"clickable,omitempty"`
	BindVideo     string       `json:"bind_video,omitempty"`     // optional video node ID for 0-IPC live playhead & scrub
	BindDuration  float64      `json:"bind_duration,omitempty"`  // media duration in seconds for bound playhead
	BindPos       float64      `json:"bind_pos,omitempty"`       // fallback position in seconds
	BindMin       float64      `json:"bind_min,omitempty"`       // optional scrub/playhead lower bound in seconds
	BindMax       float64      `json:"bind_max,omitempty"`       // optional scrub/playhead upper bound in seconds (active when BindMax > BindMin)
	PlayheadWidth int          `json:"playhead_width,omitempty"` // optional live playhead shaft width (default 4 DIP)
	Ops           []UICanvasOp `json:"ops,omitempty"`
}

// DecodeTree decodes and validates a UITree payload from raw JSON bytes.
func DecodeTree(data []byte) (UITree, error) {
	var tree UITree
	if len(data) == 0 || len(data) > MaxMessageBytes || !utf8.Valid(data) {
		return tree, ErrUITree
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tree); err != nil {
		return tree, ErrUITree
	}
	if err := tree.Validate(); err != nil {
		return UITree{}, err
	}
	return tree, nil
}

// DecodeProps decodes a node's Props map into one of the typed prop structs,
// rejecting unknown fields.
func DecodeProps[T any](n UINode) (T, error) {
	var out T
	if len(n.Props) == 0 {
		return out, nil
	}
	raw, err := json.Marshal(n.Props)
	if err != nil {
		return out, ErrUITree
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, ErrUITree
	}
	return out, nil
}

func (t UITree) Validate() error {
	if t.Rev == 0 {
		return ErrUITree
	}
	seen := make(map[string]bool)
	count := 0
	canvasOps := 0
	return validateUINode(t.Root, 1, &count, &canvasOps, seen)
}

func validateUINode(n UINode, depth int, count *int, canvasOps *int, seen map[string]bool) error {
	if depth > MaxUIDepth {
		return ErrUITree
	}
	*count++
	if *count > MaxUINodes {
		return ErrUITree
	}
	if !uiNodeIDPattern.MatchString(n.ID) || seen[n.ID] {
		return ErrUITree
	}
	seen[n.ID] = true

	switch n.Kind {
	case UINodeColumn, UINodeRow, UINodeScroll, UINodeStack:
		p, err := DecodeProps[UIContainerProps](n)
		if err != nil || !validContainerProps(p) {
			return ErrUITree
		}
	case UINodeText:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UITextProps](n)
		if err != nil || !utf8.ValidString(p.Text) || len(p.Text) > MaxUITextBytes ||
			bytes.IndexByte([]byte(p.Text), 0) >= 0 ||
			p.FontSize < 0 || p.FontSize > 96 || p.FontWeight < 0 || p.FontWeight > 900 ||
			p.PaddingV < 0 || p.PaddingV > 128 || p.PaddingH < 0 || p.PaddingH > 128 ||
			!textOK(p.Tooltip, 256, false) ||
			(p.BindVideoTime != "" && !uiNodeIDPattern.MatchString(p.BindVideoTime)) ||
			!finite64(p.BindDuration) || p.BindDuration < 0 || p.BindDuration > 86400*7 {
			return ErrUITree
		}
		switch p.Variant {
		case "", "title", "heading", "body", "muted", "accent", "danger", "success", "on_field":
		default:
			return ErrUITree
		}
		switch p.Align {
		case "", "left", "center", "right":
		default:
			return ErrUITree
		}
	case UINodeButton:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UIButtonProps](n)
		if err != nil || !textOK(p.Label, 120, p.Icon == "" && p.Variant != "swatch") || !validButtonProps(p) {
			return ErrUITree
		}
	case UINodeTextInput:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UITextInputProps](n)
		if err != nil || !utf8.ValidString(p.Value) || len(p.Value) > MaxUITextBytes ||
			bytes.IndexByte([]byte(p.Value), 0) >= 0 || !textOK(p.Placeholder, 256, false) ||
			p.Width < 0 || p.Width > 4096 {
			return ErrUITree
		}
	case UINodeImage:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UIImageProps](n)
		if err != nil || !ValidUIAssetRef(p.Src) || !textOK(p.Alt, 256, false) ||
			p.Width < 0 || p.Width > 8192 || p.Height < 0 || p.Height > 8192 ||
			p.MaxWidth < 0 || p.MaxWidth > 8192 || p.MaxHeight < 0 || p.MaxHeight > 8192 {
			return ErrUITree
		}
		switch p.Fit {
		case "", "contain", "cover":
		default:
			return ErrUITree
		}
	case UINodeVideo:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UIVideoProps](n)
		if err != nil || !ValidUIVideoSrc(p.Src) ||
			(p.Poster != "" && !ValidUIAssetRef(p.Poster)) ||
			p.Width < 0 || p.Width > 8192 || p.Height < 0 || p.Height > 8192 ||
			p.MaxWidth < 0 || p.MaxWidth > 8192 || p.MaxHeight < 0 || p.MaxHeight > 8192 ||
			!finite64(p.SeekSec) || p.SeekSec < 0 || p.SeekSec > 86400*7 ||
			!finite64(p.LoopStart) || p.LoopStart < 0 || p.LoopStart > 86400*7 ||
			!finite64(p.LoopEnd) || p.LoopEnd < 0 || p.LoopEnd > 86400*7 ||
			!finite64(p.ClampStart) || p.ClampStart < 0 || p.ClampStart > 86400*7 ||
			!finite64(p.ClampEnd) || p.ClampEnd < 0 || p.ClampEnd > 86400*7 ||
			(p.ClampEnd > 0 && p.ClampEnd < p.ClampStart) ||
			!finite64(p.Rate) || (p.Rate != 0 && (p.Rate < 0.25 || p.Rate > 4.0)) {
			return ErrUITree
		}
		if p.Volume != nil && (!finite64(*p.Volume) || *p.Volume < 0 || *p.Volume > 1) {
			return ErrUITree
		}
		switch p.Fit {
		case "", "contain", "cover":
		default:
			return ErrUITree
		}
		switch p.Surface {
		case "", "stage", "ground", "paper":
		default:
			return ErrUITree
		}
	case UINodeSplit:
		if len(n.Children) != 2 {
			return ErrUITree
		}
		p, err := DecodeProps[UISplitProps](n)
		if err != nil {
			return ErrUITree
		}
		switch p.Axis {
		case "", "h", "v":
		default:
			return ErrUITree
		}
		if !finiteNonNeg32(p.Size) || !finiteNonNeg32(p.Min) || !finiteNonNeg32(p.Max) || !finiteNonNeg32(p.DefaultSize) ||
			p.Size > 8192 || p.Min > 8192 || p.Max > 8192 || p.DefaultSize > 8192 || (p.Max > 0 && p.Min > p.Max) {
			return ErrUITree
		}
	case UINodeList:
		p, err := DecodeProps[UIListProps](n)
		if err != nil || p.Height < 0 || p.Height > 8192 {
			return ErrUITree
		}
		if p.Selected != "" && !uiNodeIDPattern.MatchString(p.Selected) {
			return ErrUITree
		}
	case UINodeProgress:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UIProgressProps](n)
		if err != nil || math.IsNaN(p.Done) || math.IsInf(p.Done, 0) ||
			math.IsNaN(p.Total) || math.IsInf(p.Total, 0) ||
			p.Done < 0 || p.Total < 0 || (p.Total > 0 && p.Done > p.Total) ||
			!textOK(p.Label, 256, false) {
			return ErrUITree
		}
	case UINodeEmpty:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UIEmptyProps](n)
		if err != nil || !textOK(p.Title, 120, true) || !textOK(p.Subtitle, 512, false) ||
			!textOK(p.ActionLabel, 120, false) ||
			(p.Icon != "" && !localIDPattern.MatchString(p.Icon)) ||
			p.IconSize < 0 || p.IconSize > 256 || p.Width < 0 || p.Width > 4096 ||
			p.BorderWidth < 0 || p.BorderWidth > 16 ||
			p.Shadow[0] < 0 || p.Shadow[0] > 32 || p.Shadow[1] < 0 || p.Shadow[1] > 32 {
			return ErrUITree
		}
		if p.PickFile != nil && !validFilePickerProps(*p.PickFile) {
			return ErrUITree
		}
	case UINodeSelect:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UISelectProps](n)
		if err != nil || !textOK(p.Label, 120, false) || !textOK(p.Value, 128, false) ||
			len(p.Options) == 0 || len(p.Options) > 128 {
			return ErrUITree
		}
		switch p.Style {
		case "", "chips", "dropdown":
		default:
			return ErrUITree
		}
		for _, opt := range p.Options {
			if !textOK(opt.Value, 128, true) || !textOK(opt.Label, 120, true) {
				return ErrUITree
			}
		}
	case UINodeToggle:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UIToggleProps](n)
		if err != nil || !textOK(p.Label, 120, true) {
			return ErrUITree
		}
		switch p.Style {
		case "", "chip":
		default:
			return ErrUITree
		}
	case UINodeTabs:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UITabsProps](n)
		if err != nil || !textOK(p.Active, 128, false) || len(p.Tabs) == 0 || len(p.Tabs) > 32 {
			return ErrUITree
		}
		for _, tab := range p.Tabs {
			if !textOK(tab.Value, 128, true) || !textOK(tab.Label, 120, true) {
				return ErrUITree
			}
		}
	case UINodeFilePicker:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UIFilePickerProps](n)
		if err != nil || !validFilePickerProps(p) {
			return ErrUITree
		}
	case UINodeDirPicker:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UIDirPickerProps](n)
		if err != nil || !textOK(p.Label, 120, true) || !textOK(p.Value, 1024, false) {
			return ErrUITree
		}
	case UINodeDraggable:
		p, err := DecodeProps[UIDraggableProps](n)
		if err != nil || !finite64(p.X) || !finite64(p.Y) || p.X < -8192 || p.X > 8192 || p.Y < -8192 || p.Y > 8192 ||
			p.Width <= 0 || p.Width > 8192 || p.Height <= 0 || p.Height > 8192 ||
			p.BoundW < 0 || p.BoundW > 8192 || p.BoundH < 0 || p.BoundH > 8192 ||
			!finite64(p.AspectRatio) || p.AspectRatio < 0 || p.AspectRatio > 100 {
			return ErrUITree
		}
		switch p.Handles {
		case "", "se", "all", "none":
		default:
			return ErrUITree
		}
	case UINodeSlider:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UISliderProps](n)
		if err != nil || !textOK(p.Label, 120, false) || !textOK(p.ValueSuffix, 16, false) ||
			p.Width < 0 || p.Width > 4096 ||
			!finite64(p.Min) || !finite64(p.Max) || !finite64(p.Step) || !finite64(p.Value) || !finite64(p.ValueScale) ||
			p.Max <= p.Min || p.Step < 0 || p.ValueScale < 0 || p.Value < p.Min || p.Value > p.Max {
			return ErrUITree
		}
	case UINodeDialog:
		p, err := DecodeProps[UIDialogProps](n)
		if err != nil || !textOK(p.Title, 120, false) || !textOK(p.Subtitle, 256, false) ||
			p.Width < 0 || p.Width > 4096 || p.MaxWidth < 0 || p.MaxWidth > 4096 ||
			p.BorderWidth < 0 || p.BorderWidth > 16 || p.Padding < 0 || p.Padding > 128 ||
			p.Shadow[0] < 0 || p.Shadow[0] > 32 || p.Shadow[1] < 0 || p.Shadow[1] > 32 {
			return ErrUITree
		}
	case UINodeIcon:
		if len(n.Children) != 0 {
			return ErrUITree
		}
		p, err := DecodeProps[UIIconProps](n)
		if err != nil || !localIDPattern.MatchString(p.Name) || p.Size < 0 || p.Size > 256 {
			return ErrUITree
		}
		switch p.Tone {
		case "", "ink", "muted", "paper", "accent", "danger":
		default:
			return ErrUITree
		}
	case UINodeCanvas:
		p, err := DecodeProps[UICanvasProps](n)
		if err != nil || p.Width <= 0 || p.Width > 8192 || p.Height <= 0 || p.Height > 8192 ||
			!finite64(p.BrushWidth) || p.BrushWidth < 0 || p.BrushWidth > 64 ||
			p.BorderWidth < 0 || p.BorderWidth > 16 ||
			p.Shadow[0] < 0 || p.Shadow[0] > 32 || p.Shadow[1] < 0 || p.Shadow[1] > 32 ||
			(p.BindVideo != "" && !uiNodeIDPattern.MatchString(p.BindVideo)) ||
			!finite64(p.BindDuration) || p.BindDuration < 0 || p.BindDuration > 86400*7 ||
			!finite64(p.BindPos) || p.BindPos < 0 || p.BindPos > 86400*7 ||
			!finite64(p.BindMin) || p.BindMin < 0 || p.BindMin > 86400*7 ||
			!finite64(p.BindMax) || p.BindMax < 0 || p.BindMax > 86400*7 ||
			(p.BindMax > 0 && p.BindMax < p.BindMin) ||
			p.PlayheadWidth < 0 || p.PlayheadWidth > 24 {
			return ErrUITree
		}
		switch p.Surface {
		case "", "paper", "ground":
		default:
			return ErrUITree
		}
		if !validColorToken(p.BrushToken) || !validBorderToken(p.BorderToken) {
			return ErrUITree
		}
		if p.Src != "" && !ValidUIAssetRef(p.Src) {
			return ErrUITree
		}
		*canvasOps += len(p.Ops)
		if *canvasOps > MaxUICanvasOps {
			return ErrUITree
		}
		for _, op := range p.Ops {
			if err := validateUICanvasOp(op); err != nil {
				return err
			}
		}
	default:
		return ErrUITree
	}

	for _, child := range n.Children {
		if err := validateUINode(child, depth+1, count, canvasOps, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateUICanvasOp(op UICanvasOp) error {
	switch op.Token {
	case "", "ink", "blue_ink", "muted", "paper", "ground", "accent", "field_support", "warning", "danger", "scrim":
	default:
		return ErrUITree
	}
	if !finite64(op.X) || !finite64(op.Y) || !finite64(op.W) || !finite64(op.H) || !finite64(op.Width) ||
		op.W < 0 || op.H < 0 || op.Width < 0 || op.Width > 256 {
		return ErrUITree
	}
	if op.Kind != "image" && op.Fit != "" {
		return ErrUITree
	}
	switch op.Kind {
	case "rect", "line":
	case "stroke_path":
		if len(op.Points) < 4 || len(op.Points)%2 != 0 || len(op.Points) > 2048 {
			return ErrUITree
		}
		for _, pt := range op.Points {
			if !finite64(pt) {
				return ErrUITree
			}
		}
	case "image":
		if !ValidUIAssetRef(op.Src) {
			return ErrUITree
		}
		switch op.Fit {
		case "", "contain", "cover", "fill":
		default:
			return ErrUITree
		}
	case "text":
		if !textOK(op.Text, 256, true) {
			return ErrUITree
		}
	default:
		return ErrUITree
	}
	return nil
}

func validContainerProps(p UIContainerProps) bool {
	if p.Gap < 0 || p.Gap > 256 || p.Padding < 0 || p.Padding > 256 ||
		p.PaddingV < 0 || p.PaddingV > 256 || p.PaddingH < 0 || p.PaddingH > 256 ||
		p.Width < 0 || p.Width > 8192 || p.Height < 0 || p.Height > 8192 ||
		p.MinWidth < 0 || p.MinWidth > 8192 || p.MinHeight < 0 || p.MinHeight > 8192 ||
		p.MaxWidth < 0 || p.MaxWidth > 8192 || p.MaxHeight < 0 || p.MaxHeight > 8192 ||
		p.BorderSize < 0 || p.BorderSize > 16 || p.Radius < 0 || p.Radius > 999 ||
		p.Shadow[0] < 0 || p.Shadow[0] > 32 || p.Shadow[1] < 0 || p.Shadow[1] > 32 ||
		len(p.Shortcuts) > 16 || !textOK(p.Tooltip, 256, false) {
		return false
	}
	for _, s := range p.BorderSides {
		if s < 0 || s > 16 {
			return false
		}
	}
	for _, sc := range p.Shortcuts {
		if !textOK(sc, 32, true) {
			return false
		}
	}
	if !validCoordPtr(p.Left) || !validCoordPtr(p.Top) || !validCoordPtr(p.Right) || !validCoordPtr(p.Bottom) {
		return false
	}
	if p.ScrollTarget != nil && (!finite64(p.ScrollTarget.Y) || p.ScrollTarget.Y < 0 || p.ScrollTarget.Y > 1e6) {
		return false
	}
	switch p.Justify {
	case "", "start", "center", "end", "between":
	default:
		return false
	}
	switch p.Align {
	case "", "start", "center", "end", "stretch":
	default:
		return false
	}
	switch p.Surface {
	case "", "ground", "paper", "field_support", "accent", "danger", "success":
	default:
		return false
	}
	return validBorderToken(p.BorderToken)
}

func validButtonProps(p UIButtonProps) bool {
	if (p.Action != "" && !localIDPattern.MatchString(p.Action)) ||
		(p.Icon != "" && !localIDPattern.MatchString(p.Icon)) ||
		p.IconSize < 0 || p.IconSize > 64 ||
		p.Width < 0 || p.Width > 1024 || p.Height < 0 || p.Height > 1024 ||
		p.PaddingX < 0 || p.PaddingX > 128 || p.PaddingY < 0 || p.PaddingY > 128 ||
		p.BorderSize < 0 || p.BorderSize > 8 ||
		!textOK(p.Tooltip, 256, false) || !textOK(p.DragPayload, 256, false) ||
		p.DragW < 0 || p.DragW > 1024 || p.DragH < 0 || p.DragH > 1024 {
		return false
	}
	switch p.Variant {
	case "", "primary", "secondary", "danger", "ghost", "swatch", "success":
	default:
		return false
	}
	if !validColorToken(p.SwatchToken) || !validBorderToken(p.BorderToken) {
		return false
	}
	if p.PickFile != nil && !validFilePickerProps(*p.PickFile) {
		return false
	}
	switch p.ShellAction {
	case "":
		if p.ShellPath != "" {
			return false
		}
	case "reveal", "open":
		if !textOK(p.ShellPath, 1024, true) {
			return false
		}
	default:
		return false
	}
	switch p.HostAction {
	case "":
		if p.HostSettingKey != "" {
			return false
		}
	case "open_app_settings":
		if p.HostSettingKey != "" {
			return false
		}
	case "run_app_setting_action":
		if !localIDPattern.MatchString(p.HostSettingKey) {
			return false
		}
	default:
		return false
	}
	if p.DragImage != "" && !ValidUIAssetRef(p.DragImage) {
		return false
	}
	return true
}

func validFilePickerProps(p UIFilePickerProps) bool {
	if !textOK(p.Label, 120, false) || !textOK(p.Value, 1024, false) || len(p.Extensions) > 32 {
		return false
	}
	switch p.Mode {
	case "", "open", "save":
	default:
		return false
	}
	for _, ext := range p.Extensions {
		if !localIDPattern.MatchString(ext) {
			return false
		}
	}
	return true
}

func validColorToken(tok string) bool {
	switch tok {
	case "", "ink", "blue_ink", "danger", "accent", "paper", "ground":
		return true
	default:
		return false
	}
}

func validBorderToken(tok string) bool {
	switch tok {
	case "", "ink", "quiet", "accent", "danger", "success":
		return true
	default:
		return false
	}
}

func validCoordPtr(v *float64) bool {
	if v == nil {
		return true
	}
	return finite64(*v) && *v >= -8192 && *v <= 8192
}

func finite64(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// ValidUIAssetRef validates an "asset:<name>" reference for live UI images.
func ValidUIAssetRef(src string) bool {
	name, ok := strings.CutPrefix(src, "asset:")
	if !ok || name == "" {
		return false
	}
	return ValidBundlePath(name)
}

// ValidUIVideoSrc validates a "file:<path>" or "asset:<name>" reference for live UI video nodes.
func ValidUIVideoSrc(src string) bool {
	if ValidUIAssetRef(src) {
		return true
	}
	path, ok := strings.CutPrefix(src, "file:")
	return ok && textOK(path, 1024, true)
}

func finiteNonNeg32(v float32) bool {
	f := float64(v)
	return !math.IsNaN(f) && !math.IsInf(f, 0) && v >= 0
}
