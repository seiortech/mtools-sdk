package pluginapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	SurfaceFile     = "surfaces.json"
	MaxSurfaceBytes = 64 << 10
)

var ErrSurface = errors.New("invalid plugin surface")

// SurfaceDocument is a signed, static description, not executable UI code.
type SurfaceDocument struct {
	Schema int           `json:"schema"`
	Tools  []ToolSurface `json:"tools"`
}

type ToolSurface struct {
	ID       ToolID          `json:"id"`
	Modes    []SurfaceMode   `json:"modes,omitempty"`
	Fields   []SurfaceField  `json:"fields"`
	Actions  []SurfaceAction `json:"actions"`
	Helpers  []SurfaceHelper `json:"helpers,omitempty"`
	Settings []SurfaceField  `json:"settings,omitempty"`
}

type SurfaceMode struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Fields []string `json:"fields"`
	Action string   `json:"action"`
}

type SurfaceHelper struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InstallURL  string `json:"install_url,omitempty"`
}

type SurfaceField struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"` // text, choice, select, toggle, files, directory; settings also allow file
	Label    string          `json:"label"`
	Required bool            `json:"required"`
	Default  string          `json:"default"`
	Options  []SurfaceOption `json:"options,omitempty"`
	Dynamic  bool            `json:"dynamic,omitempty"`
	// Description, Placeholder and Action are only meaningful for settings.
	Description string              `json:"description,omitempty"`
	Placeholder string              `json:"placeholder,omitempty"`
	Action      *SurfaceFieldAction `json:"action,omitempty"`
}

// SurfaceFieldAction attaches one button to a setting. It either opens an
// https URL through the host, or runs a declared tool action whose first
// output becomes the setting's value. It never carries executable UI code.
type SurfaceFieldAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

type SurfaceOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type SurfaceAction struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Inputs []string `json:"inputs"`
}

// InputError identifies a field without retaining or exposing its value.
type InputError struct{ Field string }

func (e *InputError) Error() string { return "Check the value for field " + e.Field + "." }

func DecodeSurfaces(data []byte, manifest Manifest) (SurfaceDocument, error) {
	var document SurfaceDocument
	if len(data) > MaxSurfaceBytes || !utf8.Valid(data) {
		return document, ErrSurface
	}
	if err := checkJSONShape(data, reflect.TypeFor[SurfaceDocument]()); err != nil {
		return document, ErrSurface
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&document); err != nil {
		return document, ErrSurface
	}
	return document, document.Validate(manifest)
}

func (d SurfaceDocument) Validate(m Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if d.Schema != 1 {
		return fmt.Errorf("%w: %w", ErrSurface, ErrCompatibility)
	}
	if len(d.Tools) == 0 || len(d.Tools) > len(m.Tools) {
		return ErrSurface
	}
	caps := func(c Capability) bool { return slices.Contains(m.Capabilities, c) }
	tools := make(map[ToolID]bool)
	for _, t := range d.Tools {
		if tools[t.ID] || !slices.ContainsFunc(m.Tools, func(v Tool) bool { return v.ID == t.ID }) ||
			len(t.Fields) > 64 || len(t.Actions) == 0 || len(t.Actions) > 16 {
			return ErrSurface
		}
		tools[t.ID] = true
		fields := make(map[string]bool)
		for _, f := range t.Fields {
			if !localIDPattern.MatchString(f.ID) || fields[f.ID] || !textOK(f.Label, 120, true) || f.Action != nil {
				return ErrSurface
			}
			fields[f.ID] = true
			if f.Kind != "choice" && f.Kind != "select" && len(f.Options) != 0 {
				return ErrSurface
			}
			switch f.Kind {
			case "text":
				if !textOK(f.Default, 8192, false) {
					return ErrSurface
				}
			case "toggle":
				if f.Default != "" && f.Default != "true" && f.Default != "false" {
					return ErrSurface
				}
			case "choice", "select":
				if !f.Dynamic && len(f.Options) == 0 {
					return ErrSurface
				}
				if len(f.Options) > 128 {
					return ErrSurface
				}
				values, labels := make(map[string]bool), make(map[string]bool)
				for _, o := range f.Options {
					if !textOK(o.Label, 120, true) || !textOK(o.Value, 128, true) || values[o.Value] || labels[o.Label] {
						return ErrSurface
					}
					values[o.Value], labels[o.Label] = true, true
				}
				if f.Default != "" && len(f.Options) > 0 && !values[f.Default] {
					return ErrSurface
				}
			case "files":
				if !caps(FilesRead) || f.Default != "" {
					return ErrSurface
				}
			case "directory":
				if !caps(FilesWrite) || f.Default != "" {
					return ErrSurface
				}
			default:
				return ErrSurface
			}
		}
		actions := make(map[string]bool)
		for _, a := range t.Actions {
			if !localIDPattern.MatchString(a.ID) || actions[a.ID] || !textOK(a.Label, 120, true) || len(a.Inputs) > 64 {
				return ErrSurface
			}
			actions[a.ID] = true
			seen := make(map[string]bool)
			for _, id := range a.Inputs {
				if !fields[id] || seen[id] {
					return ErrSurface
				}
				seen[id] = true
			}
		}
		if len(t.Modes) > 16 {
			return ErrSurface
		}
		modes := make(map[string]bool)
		for _, m := range t.Modes {
			if !localIDPattern.MatchString(m.ID) || modes[m.ID] || !textOK(m.Label, 120, true) {
				return ErrSurface
			}
			modes[m.ID] = true
			for _, fID := range m.Fields {
				if !fields[fID] {
					return ErrSurface
				}
			}
			if m.Action != "" && !actions[m.Action] {
				return ErrSurface
			}
		}
		if len(t.Helpers) > 8 {
			return ErrSurface
		}
		helpers := make(map[string]bool)
		for _, h := range t.Helpers {
			if !localIDPattern.MatchString(h.ID) || helpers[h.ID] || !textOK(h.Name, 120, true) ||
				!textOK(h.Description, 512, false) || !textOK(h.InstallURL, 1024, false) {
				return ErrSurface
			}
			helpers[h.ID] = true
		}
		if err := validateSettings(t, actions); err != nil {
			return err
		}
	}
	return nil
}

// validateSettings checks persisted per-plugin preferences. Unlike form
// fields, path settings may default to a home-relative path such as
// "~/Downloads"; the host expands it and the plugin still validates use.
func validateSettings(t ToolSurface, actions map[string]bool) error {
	if len(t.Settings) > 32 {
		return ErrSurface
	}
	seen := make(map[string]bool)
	for _, f := range t.Settings {
		if !localIDPattern.MatchString(f.ID) || seen[f.ID] || !textOK(f.Label, 120, true) ||
			!textOK(f.Description, 512, false) || !textOK(f.Placeholder, 256, false) ||
			!textOK(f.Default, 1024, false) || f.Dynamic {
			return ErrSurface
		}
		seen[f.ID] = true
		if f.Kind != "choice" && f.Kind != "select" && len(f.Options) != 0 {
			return ErrSurface
		}
		switch f.Kind {
		case "text", "file", "directory":
		case "toggle":
			if f.Default != "" && f.Default != "true" && f.Default != "false" {
				return ErrSurface
			}
		case "choice", "select":
			if len(f.Options) == 0 || len(f.Options) > 128 {
				return ErrSurface
			}
			values := make(map[string]bool)
			for _, o := range f.Options {
				if !textOK(o.Label, 120, true) || !textOK(o.Value, 128, true) || values[o.Value] {
					return ErrSurface
				}
				values[o.Value] = true
			}
			if f.Default != "" && !values[f.Default] {
				return ErrSurface
			}
		default:
			return ErrSurface
		}
		if a := f.Action; a != nil {
			if !localIDPattern.MatchString(a.ID) || !textOK(a.Label, 120, true) || !textOK(a.Description, 512, false) {
				return ErrSurface
			}
			switch {
			case a.URL != "":
				if !textOK(a.URL, 1024, true) || !strings.HasPrefix(a.URL, "https://") {
					return ErrSurface
				}
			case !actions[a.ID] || (f.Kind != "file" && f.Kind != "directory"):
				// A tool action may only fill a path setting with its output.
				return ErrSurface
			}
		}
	}
	return nil
}

// Inputs validates only the selected action's fields and copies their values.
// File existence/access is a worker boundary; this checks the portable wire shape.
func (t ToolSurface) Inputs(action string, values map[string][]string) (map[string][]string, error) {
	index := slices.IndexFunc(t.Actions, func(a SurfaceAction) bool { return a.ID == action })
	if index < 0 {
		return nil, ErrSurface
	}
	result := make(map[string][]string)
	for _, id := range t.Actions[index].Inputs {
		i := slices.IndexFunc(t.Fields, func(f SurfaceField) bool { return f.ID == id })
		if i < 0 {
			return nil, ErrSurface
		}
		f, v := t.Fields[i], values[id]
		if len(v) > 256 || (f.Kind != "files" && len(v) > 1) ||
			(f.Required && (len(v) == 0 || (len(v) == 1 && strings.TrimSpace(v[0]) == ""))) {
			return nil, &InputError{Field: id}
		}
		for _, s := range v {
			if !textOK(s, 8192, false) {
				return nil, &InputError{Field: id}
			}
			switch f.Kind {
			case "choice", "select":
				if s != "" && len(f.Options) > 0 && !slices.ContainsFunc(f.Options, func(o SurfaceOption) bool { return o.Value == s }) {
					return nil, &InputError{Field: id}
				}
			case "toggle":
				if s != "true" && s != "false" {
					return nil, &InputError{Field: id}
				}
			case "files", "directory":
				if !filepath.IsAbs(s) || filepath.Clean(s) != s {
					return nil, &InputError{Field: id}
				}
			case "text":
			default:
				return nil, ErrSurface
			}
		}
		result[id] = append([]string{}, v...)
	}
	return result, nil
}
