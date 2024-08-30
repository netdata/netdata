// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"errors"
	"os"

	"github.com/clbanning/rfile/v2"
)

const DefaultMaxLineWidth = 4 * 1024 // assume disk block size is 4K

var ErrTooLongLine = errors.New("too long line")

// ReadLastLine returns the last line of the file and any read error encountered.
// It expects last line width <= maxLineWidth.
// If maxLineWidth <= 0, it defaults to DefaultMaxLineWidth.
func ReadLastLine(filename string, maxLineWidth int64) ([]byte, error) {
	if maxLineWidth <= 0 {
		maxLineWidth = DefaultMaxLineWidth
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	stat, _ := f.Stat()
	endPos := stat.Size()
	if endPos == 0 {
		return []byte{}, nil
	}
	startPos := endPos - maxLineWidth
	if startPos < 0 {
		startPos = 0
	}
	buf := make([]byte, endPos-startPos)
	n, err := f.ReadAt(buf, startPos)
	if err != nil {
		return nil, err
	}
	lnPos := 0
	foundLn := false
	for i := n - 2; i >= 0; i-- {
		ch := buf[i]
		if ch == '\n' {
			foundLn = true
			lnPos = i
			break
		}
	}
	if foundLn {
		return buf[lnPos+1 : n], nil
	}
	if startPos == 0 {
		return buf[0:n], nil
	}

	return nil, ErrTooLongLine
}

func ReadLastLines(filename string, n uint) ([]string, error) {
	return rfile.Tail(filename, int(n))
}
