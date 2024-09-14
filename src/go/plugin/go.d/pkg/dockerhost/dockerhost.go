// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhost

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

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
		return nil, fmt.Errorf("failed to create docker client: %v", err)
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
		return nil, fmt.Errorf("failed to container exec create ('%s'): %v", container, err)
	}

	attachResp, err := cli.ContainerExecAttach(ctx, createResp.ID, typesContainer.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to container exec attach ('%s'): %v", container, err)
	}
	defer attachResp.Close()

	var outBuf, errBuf bytes.Buffer
	done := make(chan error)

	defer close(done)

	go func() {
		_, err := stdcopy.StdCopy(&outBuf, &errBuf, attachResp.Reader)
		select {
		case done <- err:
		case <-ctx.Done():
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("failed to read response from container ('%s'): %v", container, err)
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("timed out reading response")
	}

	inspResp, err := cli.ContainerExecInspect(ctx, createResp.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to container exec inspect ('%s'): %v", container, err)
	}

	if inspResp.ExitCode != 0 {
		msg := strings.ReplaceAll(errBuf.String(), "\n", " ")
		return nil, fmt.Errorf("command returned non-zero exit code (%d), error: '%s'", inspResp.ExitCode, msg)
	}

	return outBuf.Bytes(), nil
}
