// SPDX-License-Identifier: GPL-3.0-or-later

package output

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

type coordinator struct {
	primary          Writer
	secondary        Writer
	secondaryBackend Backend
	report           OutcomeReporter
}

func NewCoordinator(primary, secondary Writer, secondaryBackend Backend, report OutcomeReporter) Writer {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return &coordinator{
		primary:          primary,
		secondary:        secondary,
		secondaryBackend: secondaryBackend,
		report:           report,
	}
}

func (w *coordinator) Write(entry *model.TrapEntry) error {
	primaryErr := w.primary.Write(entry)
	if err := w.secondary.Write(entry); err != nil {
		w.report.Report(Outcome{
			Backend:       w.secondaryBackend,
			Stage:         StageEnqueue,
			FailedEntries: 1,
			Err:           err,
		})
	}
	return primaryErr
}

func (w *coordinator) Flush() error {
	primaryErr := w.primary.Flush()
	secondaryErr := w.secondary.Flush()
	return errors.Join(primaryErr, secondaryErr)
}

func (w *coordinator) Close() error {
	primaryErr := w.primary.Close()
	secondaryErr := w.secondary.Close()
	return errors.Join(primaryErr, secondaryErr)
}

func (w *coordinator) BinaryEncodedFields() uint64 {
	if binaryEncoded, ok := w.primary.(BinaryFieldCounter); ok {
		return binaryEncoded.BinaryEncodedFields()
	}
	return 0
}
