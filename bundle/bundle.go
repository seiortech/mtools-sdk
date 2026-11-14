package bundle

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/seiortech/mtools-sdk/pluginapi"
)

var (
	ErrBundle    = errors.New("invalid plugin bundle")
	ErrSignature = errors.New("plugin signature verification failed")
	ErrLimit     = errors.New("plugin bundle limit exceeded")
)

// VerifiedBundle is an immutable snapshot of a verified ZIP. It owns no open
// files and executes no code. Inspection is bounded by MaxArchiveBytes and should
// run in a worker; callers must bound concurrent inspections as well.
type VerifiedBundle struct {
	archive  []byte
	manifest []byte
	digest   string
	key      string
}

// Manifest returns a fresh copy; callers cannot mutate the verified identity,
// inventory, or capabilities by editing maps/slices returned during review.
func (b *VerifiedBundle) Manifest() pluginapi.Manifest {
	m, _ := pluginapi.DecodeManifest(b.manifest)
	return m
}

// Digest binds approval to the exact inspected archive, not a mutable file path.
func (b *VerifiedBundle) Digest() string { return b.digest }

// Fingerprint returns the hex-encoded SHA-256 digest of the publisher public key.
func (b *VerifiedBundle) Fingerprint() string { return b.key }

// ArchiveBytes returns a copy of the verified ZIP archive bytes.
func (b *VerifiedBundle) ArchiveBytes() []byte {
	if b == nil || len(b.archive) == 0 {
		return nil
	}
	out := make([]byte, len(b.archive))
	copy(out, b.archive)
	return out
}

// OpenArchive opens a read-only zip.Reader over the verified archive snapshot.
func (b *VerifiedBundle) OpenArchive() (*zip.Reader, error) {
	if b == nil || len(b.archive) == 0 {
		return nil, fmt.Errorf("%w: empty snapshot", ErrBundle)
	}
	return zip.NewReader(bytes.NewReader(b.archive), int64(len(b.archive)))
}

// InspectBundle snapshots and verifies an untrusted archive. platform is the
// host's GOOS-GOARCH. A verified self-signature is NOT publisher trust.
func InspectBundle(ctx context.Context, source io.Reader, platform string) (*VerifiedBundle, error) {
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, source}, pluginapi.MaxArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read plugin bundle: %w", err)
	}
	if len(data) > pluginapi.MaxArchiveBytes {
		return nil, ErrLimit
	}
	// Bound the central directory before archive/zip allocates per-entry state.
	// Schema 1 fits ordinary ZIP; ZIP64 and archive/file comments are unnecessary.
	if err := checkZIPDirectory(data); err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: ZIP header", ErrBundle)
	}
	// Schema 1 has only regular file entries; directories are implicit.
	if len(archive.File) < 3 || len(archive.File) > pluginapi.MaxFiles+2 {
		return nil, fmt.Errorf("%w: file count", ErrLimit)
	}
	files := make(map[string]*zip.File, len(archive.File))
	for _, f := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !pluginapi.ValidBundlePath(f.Name) || !f.Mode().IsRegular() ||
			(f.Mode().Perm() != 0644 && f.Mode().Perm() != 0755) ||
			f.Mode() & ^(fs.ModePerm) != 0 || files[f.Name] != nil {
			return nil, fmt.Errorf("%w: file name, mode or duplicate entry", ErrBundle)
		}
		if f.UncompressedSize64 > pluginapi.MaxFileBytes {
			return nil, ErrLimit
		}
		files[f.Name] = f
	}
	for _, name := range []string{"manifest.json", "SIGNATURE"} {
		if f := files[name]; f != nil && f.Mode().Perm() != 0644 {
			return nil, fmt.Errorf("%w: metadata must not be executable", ErrBundle)
		}
	}
	manifest, err := ReadZipFile(ctx, files["manifest.json"], pluginapi.MaxManifestBytes)
	if err != nil {
		return nil, err
	}
	m, err := pluginapi.DecodeManifest(manifest)
	if err != nil {
		return nil, err
	}
	signature, err := ReadZipFile(ctx, files["SIGNATURE"], ed25519.SignatureSize)
	if err != nil {
		return nil, err
	}
	key, _ := m.Publisher.Key() // DecodeManifest validated this.
	if !ed25519.Verify(key, manifest, signature) {
		return nil, ErrSignature
	}
	if m.Platform != platform {
		return nil, fmt.Errorf("%w: plugin platform does not match host", pluginapi.ErrCompatibility)
	}
	if len(files) != len(m.Files)+2 {
		return nil, fmt.Errorf("%w: payload inventory mismatch", ErrBundle)
	}
	for name, info := range m.Files {
		f := files[name]
		if f == nil || uint64(info.Size) != f.UncompressedSize64 ||
			(f.Mode().Perm() == 0755) != info.Executable {
			return nil, fmt.Errorf("%w: payload size or mode", ErrBundle)
		}
		r, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: payload reader", ErrBundle)
		}
		hash := sha256.New()
		n, copyErr := io.Copy(hash, io.LimitReader(contextReader{ctx, r}, info.Size+1))
		closeErr := r.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrBundle, copyErr)
		}
		if closeErr != nil || n != info.Size || hex.EncodeToString(hash.Sum(nil)) != info.SHA256 {
			return nil, fmt.Errorf("%w: payload checksum", ErrBundle)
		}
	}
	digest := sha256.Sum256(data)
	fingerprint, _ := m.Publisher.Fingerprint()
	return &VerifiedBundle{data, manifest, hex.EncodeToString(digest[:]), fingerprint}, nil
}

func checkZIPDirectory(data []byte) error {
	const endSize = 22
	if len(data) < endSize {
		return fmt.Errorf("%w: ZIP end record", ErrBundle)
	}
	end := data[len(data)-endSize:]
	u16, u32 := binary.LittleEndian.Uint16, binary.LittleEndian.Uint32
	if u32(end) != 0x06054b50 || u16(end[4:]) != 0 || u16(end[6:]) != 0 ||
		u16(end[8:]) != u16(end[10:]) || u16(end[20:]) != 0 {
		return fmt.Errorf("%w: ZIP end record or unsupported ZIP variant", ErrBundle)
	}
	count := int(u16(end[10:]))
	if count < 3 || count > pluginapi.MaxFiles+2 {
		return fmt.Errorf("%w: file count", ErrLimit)
	}
	start, size := uint64(u32(end[16:])), uint64(u32(end[12:]))
	stop := uint64(len(data) - endSize)
	if start+size != stop || start > stop {
		return fmt.Errorf("%w: ZIP directory bounds", ErrBundle)
	}
	cursor := start
	for i := 0; i < count; i++ {
		if cursor+46 > stop || u32(data[cursor:]) != 0x02014b50 {
			return fmt.Errorf("%w: ZIP directory entry", ErrBundle)
		}
		h := data[cursor : cursor+46]
		name, extra, comment := uint64(u16(h[28:])), uint64(u16(h[30:])), uint64(u16(h[32:]))
		if name > 240 || extra > 4096 || comment != 0 || u16(h[34:]) != 0 {
			return fmt.Errorf("%w: ZIP directory metadata", ErrLimit)
		}
		cursor += 46 + name + extra + comment
		if cursor > stop {
			return fmt.Errorf("%w: ZIP directory length", ErrBundle)
		}
	}
	if cursor != stop {
		return fmt.Errorf("%w: ZIP directory count mismatch", ErrBundle)
	}
	return nil
}

// ReadZipFile reads a bounded entry from a verified ZIP archive.
func ReadZipFile(ctx context.Context, f *zip.File, limit int64) ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("%w: required metadata missing", ErrBundle)
	}
	if f.UncompressedSize64 > uint64(limit) {
		return nil, ErrLimit
	}
	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: metadata reader", ErrBundle)
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, r}, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBundle, err)
	}
	if int64(len(data)) > limit {
		return nil, ErrLimit
	}
	return data, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// Extract writes a verified snapshot into a new, private staging root owned by
// the installer. os.Root confines operations even if a parent path is a symlink.
// Existing files are never overwritten. The installer owns cleanup on any error.
func (b *VerifiedBundle) Extract(ctx context.Context, root *os.Root) error {
	archive, err := b.OpenArchive()
	if err != nil {
		return fmt.Errorf("%w: snapshot", ErrBundle)
	}
	for _, f := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := extractFile(ctx, root, f); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(ctx context.Context, root *os.Root, f *zip.File) error {
	// Paths and modes have already been verified against the signed inventory.
	// Use slash paths converted by os.Root on the local platform.
	parent := path.Dir(f.Name)
	if parent != "." {
		if err := root.MkdirAll(parent, 0700); err != nil {
			return err
		}
	}
	out, err := root.OpenFile(f.Name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, f.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	if _, err := io.Copy(out, contextReader{ctx, in}); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}
