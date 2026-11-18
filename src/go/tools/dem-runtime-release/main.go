// SPDX-License-Identifier: GPL-3.0-or-later

// dem-runtime-release signs and verifies offline DEM release descriptors.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
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
	switch args[0] {
	case "sign":
		return sign(args[1:], diagnostics)
	case "verify":
		return verify(args[1:], diagnostics)
	case "stage-native":
		return stageNative(args[1:], diagnostics)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func sign(args []string, diagnostics io.Writer) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	fs.SetOutput(diagnostics)
	key := fs.String("key", "", "GPG signing fingerprint")
	publicKey := fs.String("public-key", "", "local public OpenPGP keyring to verify the generated signature")
	descriptor := fs.String("output", "", "new immutable signed descriptor path")
	manifest := fs.String("manifest", "", "builder manifest JSON")
	archive := fs.String("archive", "", "local runtime archive")
	sources := fs.String("sources", "", "local source archive")
	archiveURL := fs.String("archive-url", "", "immutable HTTPS runtime archive publication URL")
	sourcesURL := fs.String("sources-url", "", "immutable HTTPS source archive publication URL")
	agentVersion := fs.String("agent-version", "", "canonical paired Agent release version")
	agentChannel := fs.String("agent-channel", "", "paired Agent release channel: stable, release-candidate, nightly or devel")
	agentSource := fs.String("agent-source", "", "full lowercase paired Agent source commit SHA")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *key == "" || *descriptor == "" {
		return errors.New("key and descriptor paths are required")
	}
	if *publicKey == "" || *manifest == "" || *archive == "" || *sources == "" || *archiveURL == "" ||
		*sourcesURL == "" || *agentVersion == "" || *agentChannel == "" || *agentSource == "" {
		return errors.New(
			"sign requires public-key, manifest, archive, sources, archive-url, sources-url, agent-version, agent-channel and agent-source",
		)
	}
	agent := browserruntime.AgentRelease{
		Version: *agentVersion,
		Channel: *agentChannel,
		Source:  *agentSource,
	}
	if err := agent.Validate(); err != nil {
		return err
	}
	m, err := readBuilderManifest(*manifest)
	if err != nil {
		return err
	}
	if m.Archive.URL != "" && m.Archive.URL != *archiveURL {
		return errors.New("archive-url differs from builder manifest")
	}
	m.Archive.URL = *archiveURL
	if err := m.Validate(); err != nil {
		return err
	}
	if err := checkFile(*archive, m.Archive.SHA256, m.Archive.Bytes); err != nil {
		return fmt.Errorf("runtime archive: %w", err)
	}
	hash, size, err := hashFile(*sources)
	if err != nil {
		return fmt.Errorf("source archive: %w", err)
	}
	release := browserruntime.Release{
		Format:   2,
		Agent:    &agent,
		Manifest: m,
		Sources: browserruntime.ReleaseSource{
			URL:    *sourcesURL,
			SHA256: hash,
			Bytes:  size,
		},
	}
	if err := release.ValidateForAgent(agent); err != nil {
		return err
	}
	trusted, err := browserruntime.ReadReleaseKeyring(*publicKey)
	if err != nil {
		return err
	}
	// Accept a full fingerprint only; argument parsing must never select another
	// GPG operation or a loosely matching user ID from the publisher keyring.
	if (len(*key) != 40 && len(*key) != 64) || strings.Trim(*key, "0123456789abcdefABCDEF") != "" {
		return errors.New("sign requires a full GPG signing fingerprint")
	}
	payload, err := browserruntime.ReleaseSigningPayload(release)
	if err != nil {
		return err
	}
	command := exec.Command(
		"gpg",
		"--batch",
		"--no-tty",
		"--no-armor",
		"--no-textmode",
		"--digest-algo",
		"SHA256",
		"--local-user",
		*key,
		"--detach-sign",
		"--output",
		"-",
	)
	command.Stdin = bytes.NewReader(payload)
	// Host GPG owns private key access through GNUPGHOME; never echo its output
	// on failure, since diagnostics can contain private publisher configuration.
	signature, err := command.Output()
	if err != nil {
		return errors.New("GPG runtime release signing failed")
	}
	signed, err := browserruntime.EncodeRelease(payload, signature)
	if err != nil {
		return err
	}
	if _, err := browserruntime.VerifyRelease(signed, trusted); err != nil {
		return fmt.Errorf("verify generated signature: %w", err)
	}
	return writeNew(*descriptor, signed)
}

func verify(args []string, diagnostics io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(diagnostics)
	key := fs.String("key", "", "local public OpenPGP keyring path")
	descriptor := fs.String("release", "", "signed descriptor path")
	archive := fs.String("archive", "", "optional local runtime archive to check")
	sources := fs.String("sources", "", "optional local source archive to check")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *key == "" || *descriptor == "" {
		return errors.New("key and descriptor paths are required")
	}
	public, err := browserruntime.ReadReleaseKeyring(*key)
	if err != nil {
		return err
	}
	data, err := readBounded(*descriptor)
	if err != nil {
		return err
	}
	verified, err := browserruntime.VerifyRelease(data, public)
	if err != nil {
		return err
	}
	release := verified.Release()
	if *archive != "" {
		if err := checkFile(*archive, release.Manifest.Archive.SHA256, release.Manifest.Archive.Bytes); err != nil {
			return fmt.Errorf("runtime archive: %w", err)
		}
	}
	if *sources != "" {
		if err := checkFile(*sources, release.Sources.SHA256, release.Sources.Bytes); err != nil {
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
	data, err := io.ReadAll(io.LimitReader(f, browserruntime.ReleaseEnvelopeLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > browserruntime.ReleaseEnvelopeLimit {
		return nil, errors.New("metadata exceeds release protocol size limit")
	}
	return data, nil
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

func readBuilderManifest(path string) (browserruntime.Manifest, error) {
	var m browserruntime.Manifest
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
	return browserruntime.PrepareNativePackage(context.Background(), m, *archive, *output)
}
