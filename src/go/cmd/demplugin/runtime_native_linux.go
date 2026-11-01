// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func nativeCommandPlatform() error { return nil }

type nativeIdentity struct {
	uid, gid uint32
	groups   []uint32
	home     string
}

func nativeServiceAccount() (nativeIdentity, error) {
	var identity nativeIdentity
	account, err := user.Lookup("netdata")
	if err != nil {
		return identity, fmt.Errorf("lookup native runtime service account: %w", err)
	}
	uid, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil || uid == 0 || uid == 1<<32-1 {
		return identity, errors.New("native runtime requires a non-root netdata account")
	}
	gid, err := strconv.ParseUint(account.Gid, 10, 32)
	if err != nil || gid == 1<<32-1 {
		return identity, errors.New("native runtime service group is invalid")
	}
	if !filepath.IsAbs(account.HomeDir) {
		return identity, errors.New("native runtime service home must be absolute")
	}
	identity = nativeIdentity{uid: uint32(uid), gid: uint32(gid), home: account.HomeDir}
	groups, err := account.GroupIds()
	if err != nil {
		return identity, fmt.Errorf("lookup native runtime supplementary groups: %w", err)
	}
	for _, group := range groups {
		id, err := strconv.ParseUint(group, 10, 32)
		if err != nil || id == 1<<32-1 {
			return identity, errors.New("native runtime supplementary group is invalid")
		}
		identity.groups = append(identity.groups, uint32(id))
	}
	return identity, nil
}

type nativeChild struct {
	executable string
	identity   nativeIdentity
	diagnostic io.Writer
}

func newNativeChild(identity nativeIdentity, diagnostic io.Writer) (*nativeChild, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if err := trustedNativeExecutable(path); err != nil {
		return nil, err
	}
	return &nativeChild{executable: path, identity: identity, diagnostic: diagnostic}, nil
}

func trustedNativeExecutable(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("native service executable must be absolute")
	}
	file := true
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		owner, ok := info.Sys().(*syscall.Stat_t)
		if !ok || owner.Uid != 0 || info.Mode().Perm()&0022 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return errors.New("native service executable and ancestors must be root-owned and not group/world-writable")
		}
		if (file && (!info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0)) || (!file && !info.IsDir()) {
			return errors.New("native service executable path contains an invalid file type")
		}
		if path == "/" {
			return nil
		}
		path, file = filepath.Dir(path), false
	}
}

func nativeServiceEnvironment(identity nativeIdentity, parent []string) []string {
	environment := []string{"HOME=" + identity.home, "USER=netdata", "LOGNAME=netdata", "SHELL=/bin/sh", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}
	// Qualification uses loopback content and the build-time nd-run path. Keep
	// only local temp/cache/runtime-directory policy; no loader, Node, proxy,
	// credentials, browser-argument or executable-path injection is inherited.
	for _, entry := range parent {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		switch name {
		case "TMPDIR", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR":
			if filepath.IsAbs(value) {
				environment = append(environment, name+"="+value)
			}
		case "TZ":
			if !strings.HasPrefix(value, ":") {
				environment = append(environment, entry)
			}
		}
	}
	return environment
}

func nativeCredentialCommand(ctx context.Context, executable string, args []string, identity nativeIdentity) *exec.Cmd {
	command := exec.CommandContext(ctx, executable, args...)
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: identity.uid, Gid: identity.gid, Groups: identity.groups}}
	command.Env = nativeServiceEnvironment(identity, os.Environ())
	command.Dir = "/"
	// A killed qualifier can leave descendants holding stdio. Bound pipe joins
	// without treating that timeout as process-tree completion or releasing proof.
	command.WaitDelay = 5 * time.Second
	return command
}

func (c *nativeChild) run(ctx context.Context, verb string, input []byte) ([]byte, error) {
	switch verb {
	case "native-initialize", "native-inspect", "native-qualify":
	default:
		return nil, errors.New("invalid native service child operation")
	}
	if len(input) > nativeProtocolLimit {
		return nil, errors.New("native helper input exceeds 1 MiB")
	}
	if c.identity.uid == 0 {
		return nil, errors.New("native service child must not run as root")
	}
	command := nativeCredentialCommand(ctx, c.executable, []string{"runtime", verb}, c.identity)
	command.Stdin = bytes.NewReader(input)
	output := nativeOutput{limit: nativeProtocolLimit}
	diagnostic := nativeOutput{limit: nativeDiagnosticLimit}
	command.Stdout, command.Stderr = &output, &diagnostic
	// Arguments contain only the trusted executable and fixed internal verb.
	// Manifest input and inherited environment are never included in command logs.
	if _, err := fmt.Fprintf(c.diagnostic, "+ uid=%d gid=%d %s runtime %s\n", c.identity.uid, c.identity.gid, nativeDisplayArg(c.executable), verb); err != nil {
		return nil, fmt.Errorf("log native service command: %w", err)
	}
	err := command.Run()
	if err != nil {
		return nil, fmt.Errorf("native service %s (cwd %s): %w: %s", verb, command.Dir, err, strings.TrimSpace(diagnostic.String()))
	}
	if output.overflow {
		return nil, errors.New("native helper output exceeds 1 MiB")
	}
	return output.Bytes(), nil
}

// nativeOutput drains the complete pipe while retaining bounded diagnostics.
// Returning full write lengths avoids turning truncation into child SIGPIPE.
type nativeOutput struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *nativeOutput) Write(data []byte) (int, error) {
	original := len(data)
	remaining := b.limit - b.buffer.Len()
	if len(data) > remaining {
		b.overflow = true
		data = data[:remaining]
	}
	_, _ = b.buffer.Write(data)
	return original, nil
}

func (b *nativeOutput) Bytes() []byte  { return b.buffer.Bytes() }
func (b *nativeOutput) String() string { return b.buffer.String() }

func nativeDisplayArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
