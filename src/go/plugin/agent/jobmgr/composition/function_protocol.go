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

// functionProtocolCapture adapts one synchronous service-discovery responder
// call into a sealed result plus post-result notifications. FrameOwner remains
// the only wire writer.
type functionProtocolCapture struct {
	mu sync.Mutex

	frames *lifecycle.FrameOwner
	active *bytes.Buffer
	direct bool
}

func newFunctionProtocolCapture(
	frames *lifecycle.FrameOwner,
) (*functionProtocolCapture, error) {
	if frames == nil {
		return nil, errors.New("jobmgr composition: nil Function protocol owner")
	}
	return &functionProtocolCapture{frames: frames}, nil
}

func newFunctionProtocolCaptureWithDirectFrames(
	frames *lifecycle.FrameOwner,
) (*functionProtocolCapture, error) {
	capture, err := newFunctionProtocolCapture(frames)
	if err != nil {
		return nil, err
	}
	capture.direct = true
	return capture, nil
}

func (capture *functionProtocolCapture) Write(payload []byte) (int, error) {
	if capture == nil {
		return 0, errors.New("jobmgr composition: nil Function protocol writer")
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.active != nil {
		return capture.active.Write(payload)
	}
	if !capture.direct {
		return 0, errors.New(
			"jobmgr composition: Function protocol write outside invocation",
		)
	}
	if err := capture.frames.CommitBorrowedProtocolFrame(payload); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (capture *functionProtocolCapture) invoke(
	uid string,
	call func(),
) (lifecycle.SealedResult, lifecycle.TaskCleanup, error) {
	if capture == nil || lifecycle.ValidateUID(uid) != nil || call == nil {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: invalid Function protocol invocation")
	}
	capture.mu.Lock()
	if capture.active != nil {
		capture.mu.Unlock()
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: concurrent Function protocol invocation")
	}
	var output bytes.Buffer
	capture.active = &output
	capture.mu.Unlock()

	callErr := callFunctionProtocol(call)

	capture.mu.Lock()
	if capture.active != &output {
		capture.mu.Unlock()
		return lifecycle.SealedResult{}, nil,
			errors.Join(
				callErr,
				errors.New("jobmgr composition: Function protocol capture changed"),
			)
	}
	capture.active = nil
	capture.mu.Unlock()
	if callErr != nil {
		return lifecycle.SealedResult{}, nil, callErr
	}
	return splitFunctionProtocol(uid, output.Bytes(), capture.frames)
}

func callFunctionProtocol(call func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"%w in Function protocol handler: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			)
		}
	}()
	call()
	return nil
}

func splitFunctionProtocol(
	uid string,
	output []byte,
	frames *lifecycle.FrameOwner,
) (lifecycle.SealedResult, lifecycle.TaskCleanup, error) {
	beginPrefix := []byte("FUNCTION_RESULT_BEGIN " + uid + " ")
	begin := bytes.Index(output, beginPrefix)
	if begin < 0 {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: Function handler produced no terminal result")
	}
	headerEndRelative := bytes.IndexByte(output[begin:], '\n')
	if headerEndRelative < 0 {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: Function result has no header terminator")
	}
	headerEnd := begin + headerEndRelative
	header := strings.Fields(string(output[begin:headerEnd]))
	if len(header) != 5 ||
		header[0] != "FUNCTION_RESULT_BEGIN" ||
		header[1] != uid {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: invalid Function result header")
	}
	status, err := strconv.Atoi(header[2])
	if err != nil {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: invalid Function result status")
	}
	endMarker := []byte("\nFUNCTION_RESULT_END\n\n")
	endRelative := bytes.Index(output[headerEnd:], endMarker)
	if endRelative < 0 {
		return lifecycle.SealedResult{}, nil,
			errors.New("jobmgr composition: Function result has no terminal marker")
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
			errors.New("jobmgr composition: Function handler produced multiple results")
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
