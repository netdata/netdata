// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

// legacyProtocolCapture is the narrow residual bridge used until the secret
// and discovery domains receive their own controllers. It turns one
// legacy synchronous responder call into a sealed result plus a post-result
// protocol cleanup. The sole FrameOwner remains the only wire writer.
type legacyProtocolCapture struct {
	mu sync.Mutex

	frames *lifecycle.FrameOwner
	active *bytes.Buffer
	direct bool
}

func newLegacyProtocolCapture(
	frames *lifecycle.FrameOwner,
) (*legacyProtocolCapture, error) {
	if frames == nil {
		return nil, errors.New("jobmgr composition: nil residual protocol owner")
	}
	return &legacyProtocolCapture{frames: frames}, nil
}

func newLegacyProtocolCaptureWithDirectFrames(
	frames *lifecycle.FrameOwner,
) (*legacyProtocolCapture, error) {
	capture, err := newLegacyProtocolCapture(frames)
	if err != nil {
		return nil, err
	}
	capture.direct = true
	return capture, nil
}

func (capture *legacyProtocolCapture) Write(payload []byte) (int, error) {
	if capture == nil {
		return 0, errors.New("jobmgr composition: nil residual protocol writer")
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.active != nil {
		return capture.active.Write(payload)
	}
	if !capture.direct {
		return 0, errors.New(
			"jobmgr composition: residual protocol write outside invocation",
		)
	}
	if err := capture.frames.CommitBorrowedProtocolFrame(payload); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (capture *legacyProtocolCapture) invoke(
	uid string,
	call func(),
) (lifecycle.SealedResult, lifecycle.TaskCleanup, error) {
	if capture == nil || lifecycle.ValidateUID(uid) != nil || call == nil {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: invalid residual protocol invocation")
	}
	capture.mu.Lock()
	if capture.active != nil {
		capture.mu.Unlock()
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: concurrent residual protocol invocation")
	}
	var output bytes.Buffer
	capture.active = &output
	capture.mu.Unlock()

	callErr := callLegacyProtocol(call)

	capture.mu.Lock()
	if capture.active != &output {
		capture.mu.Unlock()
		return lifecycle.SealedResult{}, nil,
			errors.Join(
				callErr,
				errors.New("jobmgr composition: residual protocol capture changed"),
			)
	}
	capture.active = nil
	capture.mu.Unlock()
	if callErr != nil {
		return lifecycle.SealedResult{}, nil, callErr
	}
	return splitLegacyProtocol(uid, output.Bytes(), capture.frames)
}

func (capture *legacyProtocolCapture) publish(call func()) error {
	cleanup, err := capture.preparePublication(call)
	if err != nil {
		return err
	}
	return cleanup()
}

func (capture *legacyProtocolCapture) preparePublication(
	call func(),
) (lifecycle.TaskCleanup, error) {
	if capture == nil || call == nil {
		return nil, errors.New("jobmgr composition: invalid residual protocol publication")
	}
	capture.mu.Lock()
	if capture.active != nil {
		capture.mu.Unlock()
		return nil, errors.New("jobmgr composition: residual protocol publication overlaps invocation")
	}
	var output bytes.Buffer
	capture.active = &output
	capture.mu.Unlock()

	callErr := callLegacyProtocol(call)

	capture.mu.Lock()
	if capture.active != &output {
		capture.mu.Unlock()
		return nil, errors.Join(
			callErr,
			errors.New("jobmgr composition: residual protocol publication changed"),
		)
	}
	capture.active = nil
	capture.mu.Unlock()
	if callErr != nil {
		return nil, callErr
	}
	if output.Len() == 0 {
		return func() error { return nil }, nil
	}
	prepared, err := lifecycle.PrepareProtocolFrame(output.Bytes())
	if err != nil {
		return nil, err
	}
	return func() error {
		return capture.frames.CommitPreparedProtocolFrame(prepared)
	}, nil
}

func callLegacyProtocol(call func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"jobmgr composition: residual protocol handler panic: %v",
				recovered,
			)
		}
	}()
	call()
	return nil
}

func splitLegacyProtocol(
	uid string,
	output []byte,
	frames *lifecycle.FrameOwner,
) (lifecycle.SealedResult, lifecycle.TaskCleanup, error) {
	beginPrefix := []byte("FUNCTION_RESULT_BEGIN " + uid + " ")
	begin := bytes.Index(output, beginPrefix)
	if begin < 0 {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: residual handler produced no terminal result")
	}
	headerEndRelative := bytes.IndexByte(output[begin:], '\n')
	if headerEndRelative < 0 {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: residual result has no header terminator")
	}
	headerEnd := begin + headerEndRelative
	header := strings.Fields(string(output[begin:headerEnd]))
	if len(header) != 5 ||
		header[0] != "FUNCTION_RESULT_BEGIN" ||
		header[1] != uid {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: invalid residual result header")
	}
	status, err := strconv.Atoi(header[2])
	if err != nil {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: invalid residual result status")
	}
	endMarker := []byte("\nFUNCTION_RESULT_END\n\n")
	endRelative := bytes.Index(output[headerEnd:], endMarker)
	if endRelative < 0 {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: residual result has no terminal marker")
	}
	end := headerEnd + endRelative
	payload := output[headerEnd+1 : end]
	result, err := lifecycle.NewSealedResult(status, header[3], payload)
	if err != nil {
		return lifecycle.SealedResult{}, nil, err
	}
	frameEnd := end + len(endMarker)
	notifications := make(
		[]byte,
		0,
		begin+len(output)-frameEnd,
	)
	notifications = append(notifications, output[:begin]...)
	notifications = append(notifications, output[frameEnd:]...)
	if bytes.Contains(notifications, []byte("FUNCTION_RESULT_BEGIN ")) {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: residual handler produced multiple results")
	}
	cleanup := lifecycle.TaskCleanup(func() error { return nil })
	if len(notifications) != 0 {
		prepared, prepareErr := lifecycle.PrepareProtocolFrame(notifications)
		if prepareErr != nil {
			return lifecycle.SealedResult{}, nil, prepareErr
		}
		cleanup = func() error {
			return frames.CommitPreparedProtocolFrame(prepared)
		}
	}
	return result, cleanup, nil
}
