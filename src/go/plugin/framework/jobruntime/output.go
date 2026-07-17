// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"errors"
	"io"
)

type outputPoisoner interface {
	PoisonOutput(error)
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

func poisonJobOutput(writer io.Writer, err error) {
	if err == nil {
		err = errors.New("jobruntime: output state commit failed")
	}
	if poisoner, ok := writer.(outputPoisoner); ok {
		poisoner.PoisonOutput(err)
	}
}
