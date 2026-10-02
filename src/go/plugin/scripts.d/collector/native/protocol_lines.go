// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
	"errors"
)

// exchangeLines keeps the existing session reader and request ownership. Only
// successful collection replies use blocks; errors and Functions remain JSON.
func (s *scriptSession) exchangeLines(ctx context.Context, id string) (snapshot, error) {
	if err := s.write(ctx, encodeCollectRequest(id)); err != nil {
		return snapshot{}, err
	}
	var data []byte
	terminator := []byte("# EOF " + id)
	for {
		frame, err := s.read(ctx)
		if err != nil {
			return snapshot{}, err
		}
		// Frames retain CR and omit LF. Include both, and the terminator,
		// before normalization so every wire byte shares the response budget.
		if len(data)+len(frame)+1 > maxMessageBytes {
			return snapshot{}, errResponseTooLarge
		}
		line := bytes.TrimSuffix(frame, []byte{'\r'})
		if len(data) == 0 && bytes.HasPrefix(bytes.TrimSpace(line), []byte{'{'}) {
			_, err := decodeReply(line, id)
			if err == nil {
				err = errors.New("expected line snapshot, received JSON result")
			}
			return snapshot{}, err
		}
		if bytes.Equal(line, terminator) {
			return decodeLines(data)
		}
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("# EOF")) {
			return snapshot{}, errors.New("unexpected line snapshot terminator")
		}
		data = append(data, frame...)
		data = append(data, '\n')
	}
}
