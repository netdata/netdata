// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"errors"
	"io"
)

type outputPoisoner interface {
	PoisonOutput(error)
}

// OutputStateTransaction is the in-memory half of one output frame. Commit is
// called only after the complete frame is written; Abort settles every other
// path.
type OutputStateTransaction interface {
	Commit() error
	Abort() error
}

type outputTransaction interface {
	CommitJobOutput([]byte, OutputStateTransaction) error
}

func commitJobOutput(writer io.Writer, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	count, err := writer.Write(payload)
	if err == nil && count != len(payload) {
		err = io.ErrShortWrite
	}
	if err != nil {
		poisonJobOutput(writer, err)
		return err
	}
	return nil
}

func commitJobOutputTransaction(
	writer io.Writer,
	payload []byte,
	state OutputStateTransaction,
) error {
	if state == nil {
		return errors.New("jobruntime: invalid output transaction")
	}
	if len(payload) == 0 {
		if err := state.Commit(); err != nil {
			return errors.Join(err, state.Abort())
		}
		return nil
	}
	if transaction, ok := writer.(outputTransaction); ok {
		return transaction.CommitJobOutput(payload, state)
	}
	count, err := writer.Write(payload)
	if err == nil && count != len(payload) {
		err = io.ErrShortWrite
	}
	if err != nil {
		resultErr := errors.Join(err, state.Abort())
		poisonJobOutput(writer, resultErr)
		return resultErr
	}
	if err := state.Commit(); err != nil {
		resultErr := errors.Join(err, state.Abort())
		poisonJobOutput(writer, resultErr)
		return resultErr
	}
	return nil
}

func poisonJobOutput(writer io.Writer, err error) {
	if err == nil {
		err = errors.New("jobruntime: output state commit failed")
	}
	if poisoner, ok := writer.(outputPoisoner); ok {
		poisoner.PoisonOutput(err)
	}
}
