// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bufio"
	"os"
)

type input interface {
	lines() chan string
}

var stdinInput = func() input {
	r := &stdinReader{
		linesCh: make(chan string),
	}

	go r.run()

	return r
}()

type stdinReader struct {
	linesCh chan string
}

func (in *stdinReader) run() {
	sc := bufio.NewScanner(bufio.NewReader(os.Stdin))

	for sc.Scan() {
		in.linesCh <- sc.Text()
	}
}

func (in *stdinReader) lines() chan string {
	return in.linesCh
}
