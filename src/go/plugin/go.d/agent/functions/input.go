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
	r := &stdinReader{chLines: make(chan string)}
	go r.run()
	return r
}()

type stdinReader struct {
	chLines chan string
}

func (in *stdinReader) run() {
	sc := bufio.NewScanner(bufio.NewReader(os.Stdin))

	for sc.Scan() {
		text := sc.Text()
		in.chLines <- text
	}
}

func (in *stdinReader) lines() chan string {
	return in.chLines
}
