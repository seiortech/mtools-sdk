package pluginapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"unicode/utf8"
)

const (
	MaxMessageBytes  = 1 << 20 // includes the terminating LF
	MethodInitialize = "initialize"
	MethodInspect    = "inspect"
	MethodRun        = "run"
	MethodProgress   = "progress"
	MethodCancel     = "cancel"
)

var ErrProtocol = errors.New("invalid plugin protocol")

// Message is a deliberately small JSON-RPC 2.0 subset. IDs are positive integers;
// notifications omit ID. Batches, reverse requests and arbitrary error data are
// not supported. Stdout carries only LF-terminated messages.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error deliberately omits plugin-supplied text: it may contain private inputs.
func (e *RPCError) Error() string { return fmt.Sprintf("plugin request failed (code %d)", e.Code) }

type InitializeParams struct {
	Protocol string   `json:"protocol"`
	PluginID PluginID `json:"plugin_id"`
	Version  string   `json:"version"`
}

type InitializeResult = InitializeParams

type InspectParams struct {
	Tool     ToolID              `json:"tool"`
	Action   string              `json:"action,omitempty"`
	Inputs   map[string][]string `json:"inputs"`
	Settings map[string]string   `json:"settings,omitempty"`
}

// SharedTools returns a SharedToolsProvider backed by p.Settings and
// $MTOOLS_SHARED_TOOLS_DIR.
func (p InspectParams) SharedTools() SharedToolsProvider {
	return NewSharedTools(p.Settings)
}

// SharedToolsDir returns the effective default shared tools directory.
func (p InspectParams) SharedToolsDir() string {
	return p.SharedTools().SharedToolsDir()
}

type InspectPreview struct {
	Title        string   `json:"title"`
	Subtitle     string   `json:"subtitle,omitempty"`
	Meta         []string `json:"meta,omitempty"`
	ThumbnailURL string   `json:"thumbnail_url,omitempty"`
	Dismissible  bool     `json:"dismissible,omitempty"`
}

type InspectResult struct {
	Fields         map[string]InspectFieldUpdate `json:"fields,omitempty"`
	Metadata       map[string]string             `json:"metadata,omitempty"`
	Preview        *InspectPreview               `json:"preview,omitempty"`
	IsPlaylist     bool                          `json:"is_playlist,omitempty"`
	CommandPreview string                        `json:"command_preview,omitempty"`
	HelperStatus   map[string]HelperStatus       `json:"helper_status,omitempty"`
	Logs           []string                      `json:"logs,omitempty"`
	Error          string                        `json:"error,omitempty"`
}

type InspectFieldUpdate struct {
	Options []SurfaceOption `json:"options,omitempty"`
	Value   string          `json:"value,omitempty"`
	Info    string          `json:"info,omitempty"`
}

type HelperStatus struct {
	Found   bool   `json:"found"`
	Needed  bool   `json:"needed,omitempty"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Message string `json:"message,omitempty"`
}

type RunParams struct {
	JobID    string              `json:"job_id"`
	Tool     ToolID              `json:"tool"`
	Action   string              `json:"action"`
	Inputs   map[string][]string `json:"inputs"`
	Settings map[string]string   `json:"settings"`
}

// SharedTools returns a SharedToolsProvider backed by p.Settings and
// $MTOOLS_SHARED_TOOLS_DIR.
func (p RunParams) SharedTools() SharedToolsProvider {
	return NewSharedTools(p.Settings)
}

// SharedToolsDir returns the effective default shared tools directory.
func (p RunParams) SharedToolsDir() string {
	return p.SharedTools().SharedToolsDir()
}

type Progress struct {
	JobID   string  `json:"job_id"`
	Done    float64 `json:"done"`
	Total   float64 `json:"total"`
	Unit    string  `json:"unit"`
	Message string  `json:"message"`
	LogLine string  `json:"log_line,omitempty"`
}

type RunResult struct {
	JobID   string   `json:"job_id"`
	Outputs []string `json:"outputs"`
	Summary string   `json:"summary"`
}

type CancelParams struct {
	JobID string `json:"job_id"`
}

// DecodePayload applies the same strict JSON rules to a typed wire payload.
// Callers must then validate semantic bounds and job/manifest identity.
func DecodePayload(data []byte, dst any) error {
	t := reflect.TypeOf(dst)
	if t == nil || t.Kind() != reflect.Pointer || reflect.ValueOf(dst).IsNil() {
		return ErrProtocol
	}
	if len(data) == 0 || len(data) >= MaxMessageBytes || !utf8.Valid(data) {
		return ErrProtocol
	}
	if err := checkJSONShape(data, t.Elem()); err != nil {
		return fmt.Errorf("%w: JSON shape", ErrProtocol)
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return fmt.Errorf("%w: JSON value", ErrProtocol)
	}
	return nil
}

func DecodeMessage(data []byte) (Message, error) {
	var m Message
	if err := DecodePayload(data, &m); err != nil {
		return m, err
	}
	if m.JSONRPC != "2.0" {
		return m, ErrProtocol
	}
	if m.ID == 0 {
		var fields map[string]json.RawMessage
		// Shape and syntax have already been validated above.
		if err := json.Unmarshal(data, &fields); err != nil {
			return m, ErrProtocol
		}
		if _, present := fields["id"]; present {
			return m, ErrProtocol // notifications omit ID; explicit zero is not one
		}
	}
	if m.Method != "" {
		if !textOK(m.Method, 64, true) || len(m.Params) == 0 || len(m.Result) != 0 || m.Error != nil {
			return m, ErrProtocol
		}
	} else if m.ID == 0 || len(m.Params) != 0 || (len(m.Result) == 0) == (m.Error == nil) {
		return m, ErrProtocol
	}
	if m.Error != nil && (!textOK(m.Error.Message, 512, true) || m.Error.Code == 0) {
		return m, ErrProtocol
	}
	return m, nil
}

// ReadMessage requires the LF delimiter even at EOF, and bounds allocation
// before decoding. The caller owns the reader and its cancellation mechanism.
func ReadMessage(r *bufio.Reader) (Message, error) {
	var data []byte
	for len(data) < MaxMessageBytes {
		// ReadByte uses bufio's existing buffer. ReadSlice would wait for a
		// delimiter/full buffer even after receiving the first over-limit byte.
		b, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) && len(data) != 0 {
				return Message{}, ErrProtocol
			}
			return Message{}, err
		}
		if b == '\n' {
			return DecodeMessage(data)
		}
		data = append(data, b)
	}
	return Message{}, ErrProtocol
}

func WriteMessage(w io.Writer, m Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return ErrProtocol
	}
	if _, err := DecodeMessage(data); err != nil || len(data)+1 > MaxMessageBytes {
		return ErrProtocol
	}
	data = append(data, '\n')
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

func (p InitializeParams) Validate() error {
	if p.Protocol != Protocol || !ValidPluginID(p.PluginID) {
		return ErrProtocol
	}
	if _, err := CompareVersions(p.Version, p.Version); err != nil {
		return ErrProtocol
	}
	return nil
}

func ValidJobID(id string) bool {
	return localIDPattern.MatchString(id)
}

func (p RunParams) Validate() error {
	if !ValidJobID(p.JobID) || !localIDPattern.MatchString(string(p.Tool)) ||
		!localIDPattern.MatchString(p.Action) || p.Inputs == nil || p.Settings == nil ||
		len(p.Inputs) > 128 || len(p.Settings) > 128 {
		return ErrProtocol
	}
	total := 0
	for name, values := range p.Inputs {
		if !localIDPattern.MatchString(name) || values == nil || len(values) > 256 {
			return ErrProtocol
		}
		for _, value := range values {
			if !utf8.ValidString(value) || len(value) > 8192 || bytes.IndexByte([]byte(value), 0) >= 0 {
				return ErrProtocol
			}
			total += len(value)
			if total > MaxMessageBytes {
				return ErrProtocol
			}
		}
	}
	for name, value := range p.Settings {
		if !localIDPattern.MatchString(name) || !utf8.ValidString(value) || len(value) > 8192 ||
			bytes.IndexByte([]byte(value), 0) >= 0 {
			return ErrProtocol
		}
		total += len(value)
		if total > MaxMessageBytes {
			return ErrProtocol
		}
	}
	// Bound aggregate encoded size as well as each value, before launching any
	// action. Reserve space for the JSON-RPC envelope (including a 64-bit ID).
	raw, err := json.Marshal(p)
	if err != nil || len(raw)+128 > MaxMessageBytes {
		return ErrProtocol
	}
	return nil
}

func (p InspectParams) Validate() error {
	if !localIDPattern.MatchString(string(p.Tool)) {
		return ErrProtocol
	}
	if p.Action != "" && !localIDPattern.MatchString(p.Action) {
		return ErrProtocol
	}
	if len(p.Inputs) > 64 || len(p.Settings) > 64 {
		return ErrProtocol
	}
	total := 0
	for name, values := range p.Inputs {
		if !localIDPattern.MatchString(name) || values == nil || len(values) > 256 {
			return ErrProtocol
		}
		for _, value := range values {
			if !utf8.ValidString(value) || len(value) > 8192 || bytes.IndexByte([]byte(value), 0) >= 0 {
				return ErrProtocol
			}
			total += len(value)
			if total > MaxMessageBytes {
				return ErrProtocol
			}
		}
	}
	for name, value := range p.Settings {
		if !localIDPattern.MatchString(name) || !utf8.ValidString(value) || len(value) > 8192 ||
			bytes.IndexByte([]byte(value), 0) >= 0 {
			return ErrProtocol
		}
		total += len(value)
		if total > MaxMessageBytes {
			return ErrProtocol
		}
	}
	return nil
}

func (r InspectResult) Validate() error {
	if len(r.Fields) > 64 || len(r.Metadata) > 64 || len(r.HelperStatus) > 16 || len(r.Logs) > 512 {
		return ErrProtocol
	}
	if len(r.CommandPreview) > 8192 || !textOK(r.Error, 512, false) {
		return ErrProtocol
	}
	for _, l := range r.Logs {
		if !textOK(l, 2048, false) {
			return ErrProtocol
		}
	}
	return nil
}

func (p Progress) Validate(jobID string) error {
	if p.JobID != jobID || !ValidJobID(p.JobID) || math.IsNaN(p.Done) || math.IsInf(p.Done, 0) ||
		math.IsNaN(p.Total) || math.IsInf(p.Total, 0) || p.Done < 0 || p.Total < 0 ||
		(p.Total > 0 && p.Done > p.Total) || !textOK(p.Unit, 32, false) || !textOK(p.Message, 512, false) ||
		!textOK(p.LogLine, 2048, false) {
		return ErrProtocol
	}
	return nil
}

func (r RunResult) Validate(jobID string) error {
	if r.JobID != jobID || !ValidJobID(r.JobID) || r.Outputs == nil || len(r.Outputs) > 256 ||
		!textOK(r.Summary, 2048, true) {
		return ErrProtocol
	}
	for _, output := range r.Outputs {
		if !textOK(output, 8192, true) {
			return ErrProtocol
		}
	}
	return nil
}
