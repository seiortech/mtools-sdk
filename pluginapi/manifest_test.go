package pluginapi

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func validManifest() Manifest {
	return Manifest{
		Schema: SchemaVersion, ID: "org.example.echo", Name: "Echo", Version: "1.0.0",
		Platform:     "linux-amd64",
		Publisher:    Publisher{Name: "Example", PublicKey: base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))},
		Capabilities: []Capability{Process},
		Entry:        Entry{Kind: "sidecar", Path: "bin/echo", Protocol: Protocol},
		Tools:        []Tool{{ID: "echo", Name: "Echo", Category: "Examples"}},
		Files:        map[string]FileInfo{"bin/echo": {SHA256: strings.Repeat("a", 64), Size: 10, Executable: true}},
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := validManifest()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != m.ID || got.Files["bin/echo"] != m.Files["bin/echo"] {
		t.Fatalf("round trip: %#v", got)
	}
}

func TestManifestValidation(t *testing.T) {
	tests := map[string]func(*Manifest){
		"schema":               func(m *Manifest) { m.Schema++ },
		"native":               func(m *Manifest) { m.Entry.Kind = "native" },
		"protocol":             func(m *Manifest) { m.Entry.Protocol = "other" },
		"id":                   func(m *Manifest) { m.ID = "../../escape" },
		"name":                 func(m *Manifest) { m.Name = "\nforged log" },
		"version":              func(m *Manifest) { m.Version = "01.0.0" },
		"version overflow":     func(m *Manifest) { m.Version = "4294967296.0.0" },
		"platform":             func(m *Manifest) { m.Platform = "other-amd64" },
		"key":                  func(m *Manifest) { m.Publisher.PublicKey = "abc" },
		"capability":           func(m *Manifest) { m.Capabilities = []Capability{"root"} },
		"duplicate cap":        func(m *Manifest) { m.Capabilities = []Capability{Process, Process} },
		"missing process":      func(m *Manifest) { m.Capabilities = nil },
		"tools":                func(m *Manifest) { m.Tools = nil },
		"duplicate tool":       func(m *Manifest) { m.Tools = append(m.Tools, m.Tools[0]) },
		"missing entry":        func(m *Manifest) { m.Entry.Path = "bin/missing" },
		"metadata payload":     func(m *Manifest) { m.Files["SIGNATURE"] = m.Files["bin/echo"] },
		"case alias":           func(m *Manifest) { m.Files["Bin/Echo"] = m.Files["bin/echo"] },
		"parent alias":         func(m *Manifest) { m.Files["BIN"] = m.Files["bin/echo"] },
		"directory case alias": func(m *Manifest) { m.Files["BIN/other"] = m.Files["bin/echo"] },
		"traversal":            func(m *Manifest) { m.Files["../escape"] = m.Files["bin/echo"] },
		"large file": func(m *Manifest) {
			f := m.Files["bin/echo"]
			f.Size = MaxFileBytes + 1
			m.Files["bin/echo"] = f
		},
		"digest": func(m *Manifest) {
			f := m.Files["bin/echo"]
			f.SHA256 = strings.Repeat("A", 64)
			m.Files["bin/echo"] = f
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			m := validManifest()
			mutate(&m)
			if err := m.Validate(); err == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
}

func TestStrictManifestJSON(t *testing.T) {
	data, _ := json.Marshal(validManifest())
	for name, bad := range map[string][]byte{
		"duplicate":         []byte(`{"schema":1,` + string(data[1:])),
		"nested duplicate":  []byte(strings.Replace(string(data), `"kind":"sidecar"`, `"kind":"sidecar","kind":"native"`, 1)),
		"escaped duplicate": []byte(`{"\u0073chema":1,` + string(data[1:])),
		"case alias":        []byte(strings.Replace(string(data), `"schema"`, `"SCHEMA"`, 1)),
		"nested case alias": []byte(strings.Replace(string(data), `"kind"`, `"KIND"`, 1)),
		"null scalar":       []byte(strings.Replace(string(data), `"description":""`, `"description":null`, 1)),
		"unknown":           []byte(`{"unexpected":true,` + string(data[1:])),
		"trailing":          append(append([]byte{}, data...), []byte(`{}`)...),
		"oversized":         []byte(strings.Repeat(" ", MaxManifestBytes+1)),
		"depth":             []byte(strings.Repeat("[", 20) + "0" + strings.Repeat("]", 20)),
		"encoding":          {0xff},
		"null":              []byte(`null`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeManifest(bad); err == nil {
				t.Fatal("accepted invalid JSON")
			}
		})
	}
}

func TestPortablePaths(t *testing.T) {
	for _, path := range []string{
		"", ".", "..", "../x", "/root", `C:\x`, "C:/x", "bin//x", "bin/./x",
		"bin/../x", "bin/x/", "bin/x.", "bin/x ", "bin/a:b", "bin/\x00x",
		"bin/con", "bin/CON.exe", "bin/lpt1.txt", "bin/com0", "bin/écho",
		".hidden", "bin/.hidden",
	} {
		if ValidBundlePath(path) {
			t.Errorf("accepted %q", path)
		}
	}
	for _, path := range []string{"LICENSE", "assets/icon.svg", "bin/plugin.exe", "bin/plugin-linux_arm64"} {
		if !ValidBundlePath(path) {
			t.Errorf("rejected %q", path)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0}, {"1.10.0", "1.2.0", 1},
		{"0.9.9", "1.0.0", -1}, {"2.0.1", "2.0.2", -1},
	} {
		got, err := CompareVersions(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("%s %s: %d %v", tc.a, tc.b, got, err)
		}
	}
	if _, err := CompareVersions("1.0.0-beta", "1.0.0"); !errors.Is(err, ErrManifest) {
		t.Fatal(err)
	}
}

func FuzzDecodeManifest(f *testing.F) {
	data, err := json.Marshal(validManifest())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`{"schema":1,"schema":2}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := DecodeManifest(data)
		if err != nil {
			return
		}
		if err := m.Validate(); err != nil {
			t.Fatalf("decoder accepted invalid manifest: %v", err)
		}
		encoded, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeManifest(encoded); err != nil {
			t.Fatalf("accepted manifest cannot round trip: %v", err)
		}
	})
}
