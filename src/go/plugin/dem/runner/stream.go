// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// diagnosticStream owns one byte stream, independent of callback/read boundaries.
// Its exact-match carry is bounded by the longest supplied secret plus one read;
// its line buffer uses the existing text limit. Overflow discards through newline.
// Neither retained-event exhaustion nor EOF may flush an unresolved secret prefix.
type diagnosticStream struct {
	redact     *textRedactor
	emit       func(string)
	pending    bytes.Buffer
	utf8Tail   string
	line       []rune
	window     [11]byte
	dropping   bool
	pem        bool
	pemEnd     bool
	bearerNext bool
	ended      bool
	pemName    int
	pemDashes  int
}

func newDiagnosticStream(r *textRedactor, emit func(string)) *diagnosticStream {
	return &diagnosticStream{redact: r, emit: emit}
}
func (d *diagnosticStream) write(value string) {
	if d.ended {
		return
	}
	for len(value) > 0 {
		n := min(len(value), 4096)
		d.pending.WriteString(value[:n])
		value = value[n:]
		d.drain(false)
	}
}
func (d *diagnosticStream) end() {
	if d.ended {
		return
	}
	d.ended = true
	d.drain(true)
	if d.utf8Tail != "" {
		d.character(utf8.RuneError)
		d.utf8Tail = ""
	}
	if len(d.line) > 0 {
		d.flushLine()
	}
}
func (d *diagnosticStream) drain(final bool) {
	pending := d.pending.Bytes()
	safe := max(0, len(pending)-d.redact.longest+1)
	if final {
		safe = len(pending)
	}
	partial := 0
	if final {
		for _, secret := range d.redact.known {
			partial = max(partial, secretPrefixSuffix(pending, secret))
		}
	}
	partialStart := len(pending) - partial
	consumed := 0
	for consumed < safe {
		index, length := -1, 0
		for _, secret := range d.redact.known {
			if at := bytes.Index(pending[consumed:], []byte(secret)); at >= 0 && (index < 0 || at < index) {
				index, length = at, len(secret)
			}
		}
		if index < 0 || consumed+index >= min(safe, partialStart) {
			break
		}
		d.decoded(string(pending[consumed : consumed+index]))
		d.decoded("[REDACTED]")
		consumed += index + length
		if partial > 0 && consumed > partialStart {
			consumed = len(pending)
			break
		}
	}
	end := max(safe, consumed)
	if partial > 0 && consumed <= partialStart {
		d.decoded(string(pending[consumed:partialStart]))
		d.decoded("[REDACTED]")
	} else {
		d.decoded(string(pending[consumed:end]))
	}
	d.pending.Next(end)
}

// Linear suffix matching avoids quadratic work for long repeated-prefix secrets.
func secretPrefixSuffix(value []byte, secret string) int {
	if len(value) == 0 || len(secret) < 2 {
		return 0
	}
	prefix := make([]int, len(secret))
	for i, n := 1, 0; i < len(secret); i++ {
		for n > 0 && secret[i] != secret[n] {
			n = prefix[n-1]
		}
		if secret[i] == secret[n] {
			n++
		}
		prefix[i] = n
	}
	value = value[max(0, len(value)-len(secret)+1):]
	n := 0
	for i := 0; i < len(value); i++ {
		for n > 0 && value[i] != secret[n] {
			n = prefix[n-1]
		}
		if value[i] == secret[n] {
			n++
		}
	}
	return n
}
func (d *diagnosticStream) decoded(value string) {
	value = d.utf8Tail + value
	d.utf8Tail = ""
	for len(value) > 0 {
		if !utf8.FullRuneInString(value) {
			d.utf8Tail = value
			return
		}
		r, n := utf8.DecodeRuneInString(value)
		value = value[n:]
		d.character(r)
	}
}
func (d *diagnosticStream) character(r rune) {
	// Existing generic redaction recognizes multiline PEM blocks. Suppress from
	// the BEGIN prefix through the END line, without retaining the block itself.
	copy(d.window[:], d.window[1:])
	var ascii byte
	if r >= 0 && r < utf8.RuneSelf {
		ascii = byte(r)
		if ascii >= 'A' && ascii <= 'Z' {
			ascii += 'a' - 'A'
		}
	}
	d.window[len(d.window)-1] = ascii
	if !d.pem && string(d.window[:]) == "-----begin " {
		d.pem = true
		d.pemEnd = false
		d.line = nil
		d.emit("[REDACTED]\n")
	}
	if d.pem {
		if string(d.window[2:]) == "-----end " {
			d.pemEnd = true
			d.pemName = 0
			d.pemDashes = 0
		} else if d.pemEnd {
			switch {
			case d.pemDashes == 0 && (r == ' ' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
				d.pemName++
			case d.pemName > 0 && r == '-':
				d.pemDashes++
				if d.pemDashes == 5 {
					d.pem = false
					d.dropping = true
				}
			default:
				d.pemEnd = false
			}
		}
		return
	}
	if d.dropping {
		if r == '\n' {
			d.dropping = false
		}
		return
	}
	if r == '\n' {
		d.flushLine()
		return
	}
	if len(d.line) == TextLimit {
		d.line = nil
		d.dropping = true
		d.emit("[diagnostic line exceeded 2000 characters; omitted]\n")
		return
	}
	d.line = append(d.line, r)
}
func (d *diagnosticStream) flushLine() {
	line := string(d.line)
	d.line = nil
	if d.bearerNext {
		if strings.TrimSpace(line) == "" {
			return
		}
		// Reuse the canonical pattern rather than copying its credential alphabet.
		line = d.redact.patterns.Apply("Bearer " + line)
		line = strings.TrimPrefix(line, "Bearer ")
		d.bearerNext = false
	}
	trimmed := strings.TrimRight(line, " \t\r")
	if strings.HasSuffix(strings.ToLower(trimmed), "bearer") {
		line = trimmed[:len(trimmed)-len("bearer")] + "[REDACTED]"
		d.bearerNext = true
	}
	d.emit(clip(d.redact.patterns.Apply(line)) + "\n")
}
