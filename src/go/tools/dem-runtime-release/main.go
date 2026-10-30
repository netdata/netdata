// SPDX-License-Identifier: GPL-3.0-or-later

// dem-runtime-release signs and verifies offline DEM release descriptors.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
)

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, diagnostics io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: dem-runtime-release sign|verify|stage-native [flags]")
	}
	if args[0] == "stage-native" {
		return stageNative(args[1:], diagnostics)
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(diagnostics)
	var key, descriptor, archive, sources string
	fs.StringVar(&key, "key", "", "PKCS8 Ed25519 private PEM (sign) or PKIX public PEM (verify)")
	fs.StringVar(&archive, "archive", "", "local runtime archive (required for sign; optional for verify)")
	fs.StringVar(&sources, "sources", "", "local source archive (required for sign; optional for verify)")
	var manifest, archiveURL, sourcesURL string
	var sequence uint64
	switch args[0] {
	case "sign":
		fs.StringVar(&descriptor, "output", "", "new immutable signed descriptor path")
		fs.StringVar(&manifest, "manifest", "", "builder manifest JSON")
		fs.StringVar(&archiveURL, "archive-url", "", "immutable HTTPS runtime archive publication URL")
		fs.StringVar(&sourcesURL, "sources-url", "", "immutable HTTPS source archive publication URL")
		fs.Uint64Var(&sequence, "sequence", 0, "positive publication sequence in the shared release channel")
	case "verify":
		fs.StringVar(&descriptor, "release", "", "signed descriptor path")
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if key == "" || descriptor == "" {
		return errors.New("key and descriptor paths are required")
	}
	if args[0] == "verify" {
		return verify(key, descriptor, archive, sources)
	}
	if manifest == "" || archive == "" || sources == "" || archiveURL == "" || sourcesURL == "" || sequence == 0 {
		return errors.New("sign requires manifest, archive, sources, archive-url, sources-url, and a positive sequence")
	}
	m, err := readBuilderManifest(manifest)
	if err != nil {
		return err
	}
	if m.Archive.URL != "" && m.Archive.URL != archiveURL {
		return errors.New("archive-url differs from builder manifest")
	}
	m.Archive.URL = archiveURL
	if err := m.Validate(); err != nil {
		return err
	}
	if err := checkFile(archive, m.Archive.SHA256, m.Archive.Bytes); err != nil {
		return fmt.Errorf("runtime archive: %w", err)
	}
	hash, size, err := hashFile(sources)
	if err != nil {
		return fmt.Errorf("source archive: %w", err)
	}
	release := runtime.Release{
		Format:   1,
		Sequence: sequence,
		Manifest: m,
		Sources: runtime.ReleaseSource{
			URL:    sourcesURL,
			SHA256: hash,
			Bytes:  size,
		},
	}
	block, err := readPEM(key, "PRIVATE KEY")
	if err != nil {
		return err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return errors.New("invalid PKCS8 private key")
	}
	private, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return errors.New("signing key must be Ed25519")
	}
	signed, err := runtime.SignRelease(release, private)
	if err != nil {
		return err
	}
	return writeNew(descriptor, signed)
}

func verify(key, descriptor, archive, sources string) error {
	public, err := runtime.ReadReleaseKey(key)
	if err != nil {
		return err
	}
	data, err := readBounded(descriptor)
	if err != nil {
		return err
	}
	release, err := runtime.VerifyRelease(data, []ed25519.PublicKey{public})
	if err != nil {
		return err
	}
	if archive != "" {
		if err := checkFile(archive, release.Manifest.Archive.SHA256, release.Manifest.Archive.Bytes); err != nil {
			return fmt.Errorf("runtime archive: %w", err)
		}
	}
	if sources != "" {
		if err := checkFile(sources, release.Sources.SHA256, release.Sources.Bytes); err != nil {
			return fmt.Errorf("source archive: %w", err)
		}
	}
	return nil
}

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Input metadata cannot produce a valid envelope beyond the release protocol bound.
	data, err := io.ReadAll(io.LimitReader(f, runtime.ReleaseEnvelopeLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > runtime.ReleaseEnvelopeLimit {
		return nil, errors.New("metadata exceeds release protocol size limit")
	}
	return data, nil
}

func readPEM(path, kind string) (*pem.Block, error) {
	data, err := readBounded(path)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != kind || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 || !bytes.HasPrefix(bytes.TrimSpace(data), []byte("-----BEGIN "+kind+"-----")) {
		return nil, fmt.Errorf("expected one unencrypted %s PEM block", kind)
	}
	return block, nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, errors.New("artifact must be a regular file")
	}
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	if err != nil {
		return "", 0, err
	}
	if size != info.Size() {
		return "", 0, errors.New("artifact size changed while hashing")
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func checkFile(path, expectedHash string, expectedSize int64) error {
	hash, size, err := hashFile(path)
	if err != nil {
		return err
	}
	if size != expectedSize || hash != expectedHash {
		return errors.New("artifact SHA256 or byte count differs from descriptor")
	}
	return nil
}

func writeNew(path string, data []byte) error {
	// Link a complete same-directory temporary file without replacing any existing name.
	f, err := os.CreateTemp(filepath.Dir(path), ".dem-release-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Link(f.Name(), path); err != nil {
		return fmt.Errorf("create immutable descriptor (existing outputs are never replaced): %w", err)
	}
	return nil
}

func readBuilderManifest(path string) (runtime.Manifest, error) {
	var m runtime.Manifest
	data, err := readBounded(path)
	if err != nil {
		return m, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return m, fmt.Errorf("builder manifest: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return m, errors.New("builder manifest has trailing content")
	}
	return m, m.Validate()
}

func stageNative(args []string, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("stage-native", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	manifest := flags.String("manifest", "", "trusted builder manifest JSON")
	archive := flags.String("archive", "", "matching local runtime archive")
	output := flags.String("output", "", "absolute new or empty package payload staging directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *manifest == "" || *archive == "" || *output == "" {
		return errors.New("stage-native requires manifest, archive, and output")
	}
	m, err := readBuilderManifest(*manifest)
	if err != nil {
		return err
	}
	return runtime.PrepareNativePackage(context.Background(), m, *archive, *output)
}
