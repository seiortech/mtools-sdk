package main

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/seiortech/mtools-sdk/bundle"
	"github.com/seiortech/mtools-sdk/pluginapi"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "keygen":
		err = runKeygen(os.Args[2:])
	case "pack":
		err = runPack(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: mtools-plugin <command> [arguments]")
	fmt.Println("Commands:")
	fmt.Println("  keygen   Generate an Ed25519 signing keypair")
	fmt.Println("  pack     Package and sign an external plugin into a .zip bundle")
	fmt.Println("  verify   Verify a signed plugin bundle")
}

func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	outDir := fs.String("out", ".", "Directory to write key files")
	_ = fs.Parse(args)

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate keypair: %w", err)
	}

	privBase64 := base64.StdEncoding.EncodeToString(private)
	pubBase64 := base64.StdEncoding.EncodeToString(public)

	privPath := filepath.Join(*outDir, "key.priv")
	pubPath := filepath.Join(*outDir, "key.pub")

	if err := os.WriteFile(privPath, []byte(privBase64+"\n"), 0600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}
	if err := os.WriteFile(pubPath, []byte(pubBase64+"\n"), 0644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}

	sum := sha256.Sum256(public)
	fingerprint := hex.EncodeToString(sum[:])

	fmt.Printf("Wrote private key: %s\n", privPath)
	fmt.Printf("Wrote public key:  %s\n", pubPath)
	fmt.Printf("Publisher fingerprint (SHA-256): %s\n", fingerprint)
	return nil
}

func runPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	manifestPath := fs.String("manifest", "manifest.json", "Path to manifest.json")
	keyPath := fs.String("key", "key.priv", "Path to private key file")
	surfacesPath := fs.String("surfaces", "surfaces.json", "Path to surfaces.json (optional)")
	binPath := fs.String("bin", "", "Path to entry executable binary (optional override)")
	platform := fs.String("platform", "", "Target platform, e.g. darwin-arm64 (defaults to manifest or host)")
	outFile := fs.String("out", "", "Output zip file path (required)")
	_ = fs.Parse(args)

	if *outFile == "" {
		return errors.New("-out is required")
	}

	// Read and decode private key
	keyData, err := os.ReadFile(*keyPath)
	if err != nil {
		return fmt.Errorf("read private key: %w", err)
	}
	privBytes, err := base64.StdEncoding.DecodeString(string(keyData))
	if err != nil || len(privBytes) != ed25519.PrivateKeySize {
		// Try raw bytes if not base64
		if len(keyData) == ed25519.PrivateKeySize {
			privBytes = keyData
		} else {
			return fmt.Errorf("invalid ed25519 private key in %s", *keyPath)
		}
	}
	privateKey := ed25519.PrivateKey(privBytes)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Read and validate manifest
	manifestData, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var m pluginapi.Manifest
	if err := json.Unmarshal(manifestData, &m); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	if *platform != "" {
		m.Platform = *platform
	} else if m.Platform == "" {
		m.Platform = runtime.GOOS + "-" + runtime.GOARCH
	}
	if m.Entry.Kind == "" {
		m.Entry.Kind = "sidecar"
	}
	if m.Entry.Protocol == "" {
		m.Entry.Protocol = pluginapi.Protocol
	}
	if m.Entry.Path == "" {
		m.Entry.Path = "bin/tool"
	}
	if m.Files == nil {
		m.Files = make(map[string]pluginapi.FileInfo)
	}

	m.Publisher.PublicKey = base64.StdEncoding.EncodeToString(publicKey)

	baseDir := filepath.Dir(*manifestPath)
	payloads := make(map[string][]byte)

	// Handle explicit binary override if provided
	if *binPath != "" {
		bData, err := os.ReadFile(*binPath)
		if err != nil {
			return fmt.Errorf("read entry binary %s: %w", *binPath, err)
		}
		bSum := sha256.Sum256(bData)
		m.Files[m.Entry.Path] = pluginapi.FileInfo{
			SHA256:     hex.EncodeToString(bSum[:]),
			Size:       int64(len(bData)),
			Executable: true,
		}
		payloads[m.Entry.Path] = bData
	}

	// Check declared files and compute hashes
	for relPath, info := range m.Files {
		if relPath == pluginapi.SurfaceFile || (*binPath != "" && relPath == m.Entry.Path) {
			continue
		}
		fullPath := filepath.Join(baseDir, relPath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("read declared payload %s: %w", relPath, err)
		}
		sum := sha256.Sum256(data)
		actualSHA := hex.EncodeToString(sum[:])
		if info.SHA256 != "" && info.SHA256 != actualSHA {
			return fmt.Errorf("hash mismatch for %s: manifest has %s, file has %s", relPath, info.SHA256, actualSHA)
		}
		info.SHA256 = actualSHA
		info.Size = int64(len(data))
		m.Files[relPath] = info
		payloads[relPath] = data
	}

	// Check optional surfaces.json
	if *surfacesPath != "" {
		if sData, err := os.ReadFile(*surfacesPath); err == nil {
			sum := sha256.Sum256(sData)
			m.Files[pluginapi.SurfaceFile] = pluginapi.FileInfo{
				SHA256:     hex.EncodeToString(sum[:]),
				Size:       int64(len(sData)),
				Executable: false,
			}
			payloads[pluginapi.SurfaceFile] = sData
			if _, err := pluginapi.DecodeSurfaces(sData, m); err != nil {
				return fmt.Errorf("invalid surfaces.json: %w", err)
			}
		}
	}

	if err := m.Validate(); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	// Canonical manifest bytes
	canonicalManifest, err := json.Marshal(m)
	if err != nil {
		return err
	}
	signature := ed25519.Sign(privateKey, canonicalManifest)

	payloads["manifest.json"] = canonicalManifest
	payloads["SIGNATURE"] = signature

	// Write ZIP
	if err := os.MkdirAll(filepath.Dir(*outFile), 0755); err != nil && filepath.Dir(*outFile) != "." {
		return err
	}
	zipFile, err := os.Create(*outFile)
	if err != nil {
		return fmt.Errorf("create bundle file: %w", err)
	}
	zw := zip.NewWriter(zipFile)

	for name, data := range payloads {
		header := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		header.SetMode(0644)
		if name == m.Entry.Path {
			header.SetMode(0755)
		}
		w, err := zw.CreateHeader(header)
		if err != nil {
			_ = zipFile.Close()
			return err
		}
		if _, err := w.Write(data); err != nil {
			_ = zipFile.Close()
			return err
		}
	}

	if err := zw.Close(); err != nil {
		_ = zipFile.Close()
		return err
	}
	if err := zipFile.Close(); err != nil {
		return err
	}

	fmt.Printf("Created signed bundle: %s\n", *outFile)
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	bundlePath := fs.String("bundle", "", "Path to .zip bundle (required)")
	platform := fs.String("platform", runtime.GOOS+"-"+runtime.GOARCH, "Expected target platform")
	_ = fs.Parse(args)

	if *bundlePath == "" && fs.NArg() > 0 {
		*bundlePath = fs.Arg(0)
	}
	if *bundlePath == "" {
		return errors.New("bundle path is required")
	}

	f, err := os.Open(*bundlePath)
	if err != nil {
		return fmt.Errorf("open bundle: %w", err)
	}
	defer f.Close()

	b, err := bundle.InspectBundle(context.Background(), f, *platform)
	if err != nil {
		return fmt.Errorf("bundle verification failed: %w", err)
	}

	m := b.Manifest()
	fmt.Println("✓ Bundle is valid and signature is verified")
	fmt.Printf("Plugin ID:    %s\n", m.ID)
	fmt.Printf("Name:         %s\n", m.Name)
	fmt.Printf("Version:      %s\n", m.Version)
	fmt.Printf("Publisher:    %s\n", m.Publisher.Name)
	fmt.Printf("Fingerprint:  %s\n", b.Fingerprint())
	fmt.Printf("Digest:       %s\n", b.Digest())
	fmt.Printf("Capabilities: %v\n", m.Capabilities)
	return nil
}
