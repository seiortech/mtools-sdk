package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/seiortech/mtools-sdk/pluginapi"
)

func TestMtoolsPluginCLI(t *testing.T) {
	tmp := t.TempDir()

	// 1. Keygen
	if err := runKeygen([]string{"-out", tmp}); err != nil {
		t.Fatalf("keygen: %v", err)
	}

	keyPriv := filepath.Join(tmp, "key.priv")
	keyPub := filepath.Join(tmp, "key.pub")
	if _, err := os.Stat(keyPriv); err != nil {
		t.Fatal("missing key.priv")
	}
	if _, err := os.Stat(keyPub); err != nil {
		t.Fatal("missing key.pub")
	}

	// 2. Prepare fixture directory with binary & surfaces
	pluginDir := filepath.Join(tmp, "echo-plugin")
	binDir := filepath.Join(pluginDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	binPath := filepath.Join(binDir, "echo")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := pluginapi.Manifest{
		Schema:       1,
		ID:           "com.example.test",
		Name:         "Test Plugin",
		Version:      "1.0.0",
		Platform:     runtime.GOOS + "-" + runtime.GOARCH,
		Publisher:    pluginapi.Publisher{Name: "Tester"},
		Capabilities: []pluginapi.Capability{pluginapi.Process},
		Entry:        pluginapi.Entry{Kind: "sidecar", Path: "bin/echo", Protocol: pluginapi.Protocol},
		Tools:        []pluginapi.Tool{{ID: "echo", Name: "Echo", Category: "Examples"}},
		Files: map[string]pluginapi.FileInfo{
			"bin/echo": {Executable: true},
		},
	}
	mBytes, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(manifestPath, mBytes, 0644); err != nil {
		t.Fatal(err)
	}

	surfaces := pluginapi.SurfaceDocument{
		Schema: 1,
		Tools: []pluginapi.ToolSurface{
			{
				ID: "echo",
				Fields: []pluginapi.SurfaceField{
					{ID: "text", Kind: "text", Label: "Input", Required: true},
				},
				Actions: []pluginapi.SurfaceAction{
					{ID: "echo", Label: "Echo", Inputs: []string{"text"}},
				},
			},
		},
	}
	sBytes, _ := json.Marshal(surfaces)
	surfacesPath := filepath.Join(pluginDir, "surfaces.json")
	if err := os.WriteFile(surfacesPath, sBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Pack
	bundleZip := filepath.Join(tmp, "bundle.zip")
	if err := runPack([]string{
		"-manifest", manifestPath,
		"-key", keyPriv,
		"-surfaces", surfacesPath,
		"-out", bundleZip,
	}); err != nil {
		t.Fatalf("pack: %v", err)
	}

	if _, err := os.Stat(bundleZip); err != nil {
		t.Fatal("missing bundle.zip")
	}

	// 4. Verify
	if err := runVerify([]string{"-bundle", bundleZip}); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// 5. Test -bin and -platform override
	binExternal := filepath.Join(tmp, "external-bin")
	if err := os.WriteFile(binExternal, []byte("#!/bin/sh\necho external\n"), 0755); err != nil {
		t.Fatal(err)
	}
	bundleOverrideZip := filepath.Join(tmp, "override.zip")
	manifestNoFiles := pluginapi.Manifest{
		Schema:       1,
		ID:           "com.example.override",
		Name:         "Override Plugin",
		Version:      "1.0.0",
		Publisher:    pluginapi.Publisher{Name: "Tester"},
		Capabilities: []pluginapi.Capability{pluginapi.Process},
		Entry:        pluginapi.Entry{Kind: "sidecar", Path: "bin/tool", Protocol: pluginapi.Protocol},
		Tools:        []pluginapi.Tool{{ID: "override", Name: "Override", Category: "Examples"}},
	}
	mNoFilesBytes, _ := json.Marshal(manifestNoFiles)
	manifestNoFilesPath := filepath.Join(tmp, "manifest-nofiles.json")
	if err := os.WriteFile(manifestNoFilesPath, mNoFilesBytes, 0644); err != nil {
		t.Fatal(err)
	}

	if err := runPack([]string{
		"-manifest", manifestNoFilesPath,
		"-key", keyPriv,
		"-bin", binExternal,
		"-platform", runtime.GOOS + "-" + runtime.GOARCH,
		"-out", bundleOverrideZip,
	}); err != nil {
		t.Fatalf("pack with -bin: %v", err)
	}

	if err := runVerify([]string{"-bundle", bundleOverrideZip}); err != nil {
		t.Fatalf("verify override: %v", err)
	}
}
