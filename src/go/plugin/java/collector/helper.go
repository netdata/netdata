// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/java/protocol"
)

type nativeHelper struct{}

type helper interface {
	discover(context.Context) ([]protocol.Process, error)
	attach(context.Context, protocol.AttachRequest) protocol.AttachResult
}

func (nativeHelper) discover(ctx context.Context) ([]protocol.Process, error) {
	var response protocol.Discovery
	err := callHelper(ctx, "java-discover", nil, &response)
	return response.Processes, err
}

func (nativeHelper) attach(ctx context.Context, req protocol.AttachRequest) protocol.AttachResult {
	var response protocol.AttachResult
	if err := callHelper(ctx, "java-attach", req, &response); err != nil {
		return protocol.AttachResult{Status: protocol.Unknown, Detail: "Attachment could not be acknowledged; no automatic retry for this process"}
	}
	if response.Status != protocol.Attached && response.Status != protocol.Blocked && response.Status != protocol.Unknown {
		return protocol.AttachResult{Status: protocol.Unknown, Detail: "Invalid helper outcome; no automatic retry for this process"}
	}
	return response
}

// The native supervisor owns a shorter deadline and reaps the same-UID child.
// ndsudo's caller cannot reliably signal root; no secret enters argv or logs.
func callHelper(parent context.Context, command string, request, response any) error {
	ctx, cancel := context.WithTimeout(parent, 35*time.Second)
	defer cancel()
	input, inputWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer input.Close()
	defer inputWriter.Close()
	output, outputWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer output.Close()
	defer outputWriter.Close()
	process, err := ndexec.StartNDSudoProcess(ctx, ndexec.ProcessOptions{Stdin: input, Stdout: outputWriter, Stderr: os.Stderr}, command)
	if err != nil {
		return err
	}
	_ = input.Close()
	_ = outputWriter.Close()
	written := make(chan error, 1)
	go func() {
		var err error
		if request != nil {
			err = json.NewEncoder(inputWriter).Encode(request)
		}
		_ = inputWriter.Close()
		written <- err
	}()
	decoded := make(chan error, 1)
	go func() {
		decoder := json.NewDecoder(io.LimitReader(output, 4<<20))
		err := decoder.Decode(response)
		if err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = fmt.Errorf("unexpected helper output")
			}
		}
		decoded <- err
	}()
	waited := make(chan error, 1)
	go func() { waited <- process.Wait() }()
	for _, done := range []<-chan error{written, decoded, waited} {
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("%s failed: %w", command, err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
