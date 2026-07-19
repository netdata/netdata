// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

var ErrPreparedVNodeFrameConsumed = errors.New("job output: prepared vnode frame consumed")

type PreparedVNodeFrame struct {
	state *preparedVNodeFrameState
}

type preparedVNodeFrameState struct {
	mu         sync.Mutex
	consumed   bool
	generation uint64
	revision   uint64
	frame      lifecycle.PreparedProtocolFrame
	commit     func() error
	abort      func() error
}

func PrepareVNodeFrame(
	generation uint64,
	revision uint64,
	payload []byte,
	commit func() error,
	abort func() error,
) (PreparedVNodeFrame, error) {
	if generation == 0 || revision == 0 || commit == nil || abort == nil {
		return PreparedVNodeFrame{}, errors.New("job output: invalid vnode frame transaction")
	}
	frame, err := lifecycle.PrepareProtocolFrame(payload)
	if err != nil {
		return PreparedVNodeFrame{}, err
	}
	return PreparedVNodeFrame{state: &preparedVNodeFrameState{
		generation: generation, revision: revision,
		frame: frame, commit: commit, abort: abort,
	}}, nil
}

func (pvf PreparedVNodeFrame) Transfer(owner *lifecycle.FrameOwner) error {
	if owner == nil {
		return errors.New("job output: nil FrameOwner")
	}
	state, err := pvf.take()
	if err != nil {
		return err
	}
	return owner.CommitPreparedProtocolTransaction(
		state.frame,
		state.commit,
		state.abort,
	)
}

func (pvf PreparedVNodeFrame) Abort() error {
	state, err := pvf.take()
	if err != nil {
		return err
	}
	return errors.Join(state.frame.Abort(), state.abort())
}

func (pvf PreparedVNodeFrame) Generation() uint64 {
	if pvf.state == nil {
		return 0
	}
	return pvf.state.generation
}

func (pvf PreparedVNodeFrame) Revision() uint64 {
	if pvf.state == nil {
		return 0
	}
	return pvf.state.revision
}

func (pvf PreparedVNodeFrame) take() (*preparedVNodeFrameState, error) {
	if pvf.state == nil {
		return nil, errors.New("job output: unprepared vnode frame")
	}
	pvf.state.mu.Lock()
	defer pvf.state.mu.Unlock()
	if pvf.state.consumed {
		return nil, ErrPreparedVNodeFrameConsumed
	}
	pvf.state.consumed = true
	return pvf.state, nil
}
