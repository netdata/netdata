// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bufio"
	"os"
	"sync"
)

type input interface {
	lines() chan string
}

func newStdinInput() input {
	return &stdinReader{}
}

type stdinReader struct {
	once    sync.Once
	linesCh chan string
}

func (in *stdinReader) run() {
	sc := bufio.NewScanner(bufio.NewReader(os.Stdin))

	for sc.Scan() {
		in.linesCh <- sc.Text()
	}
}

func (in *stdinReader) lines() chan string {
	in.once.Do(func() {
		in.linesCh = make(chan string)
		go in.run()
	})

	return in.linesCh
}
