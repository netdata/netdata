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

func (prepared PreparedVNodeFrame) Transfer(owner *lifecycle.FrameOwner) error {
	if owner == nil {
		return errors.New("job output: nil FrameOwner")
	}
	state, err := prepared.take()
	if err != nil {
		return err
	}
	return owner.CommitPreparedProtocolTransaction(
		state.frame,
		state.commit,
		state.abort,
	)
}

func (prepared PreparedVNodeFrame) Abort() error {
	state, err := prepared.take()
	if err != nil {
		return err
	}
	return errors.Join(state.frame.Abort(), state.abort())
}

func (prepared PreparedVNodeFrame) Generation() uint64 {
	if prepared.state == nil {
		return 0
	}
	return prepared.state.generation
}

func (prepared PreparedVNodeFrame) Revision() uint64 {
	if prepared.state == nil {
		return 0
	}
	return prepared.state.revision
}

func (prepared PreparedVNodeFrame) take() (*preparedVNodeFrameState, error) {
	if prepared.state == nil {
		return nil, errors.New("job output: unprepared vnode frame")
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	if prepared.state.consumed {
		return nil, ErrPreparedVNodeFrameConsumed
	}
	prepared.state.consumed = true
	return prepared.state, nil
}
