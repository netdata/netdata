// SPDX-License-Identifier: GPL-3.0-or-later

package safewriter

import (
	"io"
	"os"
	"sync"
)

var Stdout = New(os.Stdout)

func New(w io.Writer) io.Writer {
	return &writer{
		mx: &sync.Mutex{},
		w:  w,
	}
}

type writer struct {
	mx *sync.Mutex
	w  io.Writer
}

func (w *writer) Write(p []byte) (n int, err error) {
	w.mx.Lock()
	n, err = w.w.Write(p)
	w.mx.Unlock()
	return n, err
}
