// Package pluginapi defines the experimental external plugin bundle contract.
// It has no dependency on the desktop UI or mtools' internal packages.
// The runtime protocol is experimental; the surface schema is not frozen yet.
package pluginapi

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaVersion    = 1
	Protocol         = "jsonrpc-stdio-v1"
	MaxManifestBytes = 256 << 10
	MaxFiles         = 2048
	MaxFileBytes     = 128 << 20
	MaxExpandedBytes = 512 << 20
	MaxArchiveBytes  = 256 << 20
)

var (
	ErrManifest      = errors.New("invalid plugin manifest")
	ErrCompatibility = errors.New("incompatible plugin")
)

type PluginID string
type ToolID string
type Capability string

const (
	FilesRead  Capability = "files.read"
	FilesWrite Capability = "files.write"
	Network    Capability = "network"
	Process    Capability = "process"
	Clipboard  Capability = "clipboard"
	UILive     Capability = "ui.live"
)

// Manifest is signed as exact UTF-8 JSON bytes, not re-encoded JSON.
// Files inventories every ZIP payload except manifest.json and SIGNATURE.
type Manifest struct {
	Schema       int                 `json:"schema"`
	ID           PluginID            `json:"id"`
	Name         string              `json:"name"`
	Version      string              `json:"version"`
	Description  string              `json:"description"`
	Platform     string              `json:"platform"`
	Publisher    Publisher           `json:"publisher"`
	Capabilities []Capability        `json:"capabilities"`
	Entry        Entry               `json:"entry"`
	Tools        []Tool              `json:"tools"`
	Files        map[string]FileInfo `json:"files"`
}

type Publisher struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"` // standard padded base64, Ed25519
}

type Entry struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Protocol string `json:"protocol"`
}

type Tool struct {
	ID          ToolID `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type FileInfo struct {
	SHA256     string `json:"sha256"` // lowercase hex
	Size       int64  `json:"size"`
	Executable bool   `json:"executable"`
}

var (
	pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*\.[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	localIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	versionPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	pathPartPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`)
)

func ValidPluginID(id PluginID) bool {
	return len(id) <= 128 && pluginIDPattern.MatchString(string(id)) && ValidBundlePath(string(id))
}

// CompareVersions compares release versions. Prerelease/build suffixes are not
// supported in schema 1, so version order has no channel-dependent ambiguity.
func CompareVersions(a, b string) (int, error) {
	parse := func(v string) ([3]uint32, error) {
		var result [3]uint32
		if !versionPattern.MatchString(v) || len(v) > 32 {
			return result, fmt.Errorf("%w: version must be major.minor.patch", ErrManifest)
		}
		for i, part := range strings.Split(v, ".") {
			n, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				return result, fmt.Errorf("%w: version component overflow", ErrManifest)
			}
			result[i] = uint32(n)
		}
		return result, nil
	}
	av, err := parse(a)
	if err != nil {
		return 0, err
	}
	bv, err := parse(b)
	if err != nil {
		return 0, err
	}
	for i := range av {
		if av[i] < bv[i] {
			return -1, nil
		}
		if av[i] > bv[i] {
			return 1, nil
		}
	}
	return 0, nil
}

// ValidBundlePath accepts only portable, slash-separated regular file paths.
// Reject aliases on case-insensitive/Windows filesystems before extraction.
func ValidBundlePath(name string) bool {
	if len(name) == 0 || len(name) > 240 || path.Clean(name) != name {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if len(part) > 100 || !pathPartPattern.MatchString(part) || strings.HasSuffix(part, ".") {
			return false
		}
		base := strings.ToLower(strings.SplitN(part, ".", 2)[0])
		switch base {
		case "con", "prn", "aux", "nul", "clock$":
			return false
		}
		if len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) &&
			base[3] >= '0' && base[3] <= '9' {
			return false
		}
	}
	return true
}

func textOK(s string, max int, required bool) bool {
	if !utf8.ValidString(s) || len(s) > max || (required && strings.TrimSpace(s) == "") {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}

func (p Publisher) Key() (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.Strict().DecodeString(p.PublicKey)
	if err != nil || len(b) != ed25519.PublicKeySize ||
		base64.StdEncoding.EncodeToString(b) != p.PublicKey {
		return nil, fmt.Errorf("%w: invalid publisher key", ErrManifest)
	}
	return ed25519.PublicKey(b), nil
}

// Fingerprint identifies a key, not a verified real-world publisher identity.
func (p Publisher) Fingerprint() (string, error) {
	key, err := p.Key()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(key)
	return hex.EncodeToString(digest[:]), nil
}

// DecodeManifest rejects unknown and duplicate members, trailing JSON, excessive
// nesting, invalid UTF-8, and metadata beyond the schema's bounds.
func DecodeManifest(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) > MaxManifestBytes || !utf8.Valid(data) {
		return m, fmt.Errorf("%w: manifest size or encoding", ErrManifest)
	}
	if err := checkJSON(data); err != nil {
		return m, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return m, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	return m, m.Validate()
}

func (m Manifest) Validate() error {
	bad := func(reason string) error { return fmt.Errorf("%w: %s", ErrManifest, reason) }
	if m.Schema != SchemaVersion || m.Entry.Kind != "sidecar" || m.Entry.Protocol != Protocol {
		return fmt.Errorf("%w: schema, entry kind or protocol", ErrCompatibility)
	}
	if !ValidPluginID(m.ID) || !textOK(m.Name, 120, true) ||
		!textOK(m.Description, 2048, false) || !textOK(m.Publisher.Name, 120, true) {
		return bad("identity or display metadata")
	}
	if _, err := CompareVersions(m.Version, m.Version); err != nil {
		return err
	}
	switch m.Platform {
	case "darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64", "windows-arm64", "windows-amd64":
	default:
		return fmt.Errorf("%w: platform", ErrCompatibility)
	}
	if _, err := m.Publisher.Key(); err != nil {
		return err
	}
	caps := make(map[Capability]bool)
	for _, cap := range m.Capabilities {
		switch cap {
		case FilesRead, FilesWrite, Network, Process, Clipboard, UILive:
		default:
			return bad("unknown capability")
		}
		if caps[cap] {
			return bad("duplicate capability")
		}
		caps[cap] = true
	}
	if !caps[Process] {
		return bad("sidecar requires process capability")
	}
	if len(m.Tools) == 0 || len(m.Tools) > 32 {
		return bad("tool count")
	}
	ids := make(map[ToolID]bool)
	for _, t := range m.Tools {
		if !localIDPattern.MatchString(string(t.ID)) || ids[t.ID] ||
			!textOK(t.Name, 120, true) || !textOK(t.Description, 2048, false) ||
			!textOK(t.Category, 64, true) {
			return bad("tool metadata or duplicate ID")
		}
		ids[t.ID] = true
	}
	if len(m.Files) == 0 || len(m.Files) > MaxFiles {
		return bad("file count")
	}
	names := map[string]bool{"manifest.json": true, "signature": true}
	spelling := make(map[string]string)
	var total int64
	for name, f := range m.Files {
		key := strings.ToLower(name)
		if !ValidBundlePath(name) || names[key] {
			return bad("unsafe or duplicate file path")
		}
		for prefix := name; prefix != "."; prefix = path.Dir(prefix) {
			folded := strings.ToLower(prefix)
			if previous, exists := spelling[folded]; exists && previous != prefix {
				return bad("inconsistent path casing")
			}
			spelling[folded] = prefix
		}
		names[key] = true
		digest, err := hex.DecodeString(f.SHA256)
		if err != nil || len(digest) != sha256.Size || strings.ToLower(f.SHA256) != f.SHA256 ||
			f.Size < 0 || f.Size > MaxFileBytes {
			return bad("file digest or size")
		}
		total += f.Size
	}
	if total > MaxExpandedBytes {
		return bad("expanded size")
	}
	for name := range names {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if names[parent] {
				return bad("file/directory collision")
			}
		}
	}
	entry, found := m.Files[m.Entry.Path]
	if !ValidBundlePath(m.Entry.Path) || !found || !entry.Executable || entry.Size == 0 {
		return bad("entry must name a nonempty executable payload")
	}
	return nil
}

// checkJSON avoids parser disagreement over duplicate keys, case-insensitive
// struct field aliases, and nulls. Derive exact member names from the wire types
// rather than maintaining another copy of the schema.
func checkJSON(data []byte) error {
	return checkJSONShape(data, reflect.TypeFor[Manifest]())
}

var jsonStructFieldsCache sync.Map // reflect.Type -> map[string]reflect.Type

func cachedStructJSONFields(t reflect.Type) map[string]reflect.Type {
	if v, ok := jsonStructFieldsCache.Load(t); ok {
		return v.(map[string]reflect.Type)
	}
	fields := make(map[string]reflect.Type, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fields[strings.Split(field.Tag.Get("json"), ",")[0]] = field.Type
	}
	actual, _ := jsonStructFieldsCache.LoadOrStore(t, fields)
	return actual.(map[string]reflect.Type)
}

// checkJSONShape also validates RPC envelopes and typed payloads. RawMessage
// subtrees may have arbitrary keys, but still reject duplicates/nulls/deep JSON.
func checkJSONShape(data []byte, root reflect.Type) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var value func(int, reflect.Type) error
	value = func(depth int, shape reflect.Type) error {
		if depth > 16 {
			return errors.New("JSON nesting limit")
		}
		for shape.Kind() == reflect.Pointer {
			shape = shape.Elem()
		}
		if shape == reflect.TypeFor[json.RawMessage]() {
			shape = reflect.TypeFor[any]()
		}
		dynamic := shape.Kind() == reflect.Interface
		t, err := d.Token()
		if err != nil {
			return err
		}
		if t == nil {
			return errors.New("null JSON value")
		}
		switch t {
		case json.Delim('{'):
			if !dynamic && shape.Kind() != reflect.Struct && shape.Kind() != reflect.Map {
				return errors.New("unexpected JSON object")
			}
			var fields map[string]reflect.Type
			if shape.Kind() == reflect.Struct {
				fields = cachedStructJSONFields(shape)
			}
			keys := make(map[string]bool)
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := k.(string)
				if !ok || keys[name] {
					return errors.New("duplicate JSON member")
				}
				keys[name] = true
				var child reflect.Type
				if dynamic {
					child = shape
				} else if shape.Kind() == reflect.Map {
					child = shape.Elem()
				} else {
					child = fields[name]
					if child == nil {
						return errors.New("unknown JSON member")
					}
				}
				if err := value(depth+1, child); err != nil {
					return err
				}
			}
			_, err = d.Token()
		case json.Delim('['):
			if !dynamic && shape.Kind() != reflect.Slice {
				return errors.New("unexpected JSON array")
			}
			for d.More() {
				child := shape
				if !dynamic {
					child = shape.Elem()
				}
				if err := value(depth+1, child); err != nil {
					return err
				}
			}
			_, err = d.Token()
		}
		return err
	}
	if err := value(0, root); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
