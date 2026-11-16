// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import "fmt"

// tableProcessingErrors retains one representative failure per configured table,
// rather than retaining or formatting an error for every rejected row or value.
type tableProcessingErrors struct {
	count    int
	first    error
	firstRow string
}

func (s *tableProcessingErrors) add(row string, err error) {
	s.count++
	if s.first == nil {
		s.first = err
		s.firstRow = row
	}
}

func (s tableProcessingErrors) err() error {
	if s.count == 0 {
		return nil
	}
	return fmt.Errorf("%d processing errors; first row %q: %w", s.count, s.firstRow, s.first)
}
