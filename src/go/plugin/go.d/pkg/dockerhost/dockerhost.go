// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhost

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	typesContainer "github.com/docker/docker/api/types/container"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

func FromEnv() string {
	addr := os.Getenv("DOCKER_HOST")
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, "tcp://") || strings.HasPrefix(addr, "unix://") {
		return addr
	}
	if strings.HasPrefix(addr, "/") {
		return fmt.Sprintf("unix://%s", addr)
	}
	return fmt.Sprintf("tcp://%s", addr)
}

func Exec(ctx context.Context, container string, cmd string, args ...string) ([]byte, error) {
	// based on https://github.com/moby/moby/blob/8e610b2b55bfd1bfa9436ab110d311f5e8a74dcb/integration/internal/container/exec.go#L38
	addr := docker.DefaultDockerHost
	if v := FromEnv(); v != "" {
		addr = v
	}

	cli, err := docker.NewClientWithOpts(docker.WithHost(addr))
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	cli.NegotiateAPIVersion(ctx)

	execCreateConfig := typesContainer.ExecOptions{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          append([]string{cmd}, args...),
	}

	createResp, err := cli.ContainerExecCreate(ctx, container, execCreateConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to container exec create (%s): %w", container, err)
	}

	attachResp, err := cli.ContainerExecAttach(ctx, createResp.ID, typesContainer.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to container exec attach (%s): %w", container, err)
	}
	defer attachResp.Close()

	var outBuf, errBuf bytes.Buffer
	done := make(chan error, 1)

	go func() {
		_, err := stdcopy.StdCopy(&outBuf, &errBuf, attachResp.Reader)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("failed to read response from container (%s): %w", container, err)
		}
	case <-ctx.Done():
		// Close connection to interrupt StdCopy
		attachResp.Close()

		select {
		case <-done:
		case <-time.After(150 * time.Millisecond):
			// Don't wait too long, let it clean up in background
		}

		return nil, fmt.Errorf("timed out reading response: %w", ctx.Err())
	}

	inspResp, err := cli.ContainerExecInspect(ctx, createResp.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to container exec inspect (%s): %w", container, err)
	}

	if inspResp.ExitCode != 0 {
		msg := strings.ReplaceAll(errBuf.String(), "\n", " ")
		return nil, fmt.Errorf("command returned non-zero exit code (%d), error: %q", inspResp.ExitCode, msg)
	}

	return outBuf.Bytes(), nil
}
