// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import "errors"

type fanoutTrapWriter struct {
	primary   TrapWriter
	secondary TrapWriter
	metrics   *perJobMetrics
}

func newFanoutTrapWriter(primary, secondary TrapWriter, metrics *perJobMetrics) TrapWriter {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return &fanoutTrapWriter{
		primary:   primary,
		secondary: secondary,
		metrics:   metrics,
	}
}

func (w *fanoutTrapWriter) Write(entry *TrapEntry) error {
	if err := w.primary.Write(entry); err != nil {
		return err
	}
	if err := w.secondary.Write(entry); err != nil {
		w.incOTLPExportFailed(1)
	}
	return nil
}

func (w *fanoutTrapWriter) Flush() error {
	primaryErr := w.primary.Flush()
	secondaryErr := w.secondary.Flush()
	if secondaryErr != nil {
		w.incOTLPExportFailed(1)
	}
	return errors.Join(primaryErr, secondaryErr)
}

func (w *fanoutTrapWriter) Close() error {
	secondaryErr := w.secondary.Close()
	if secondaryErr != nil {
		w.incOTLPExportFailed(1)
	}
	return errors.Join(w.primary.Close(), secondaryErr)
}

func (w *fanoutTrapWriter) SanitizedFields() uint64 {
	if sanitized, ok := w.primary.(interface{ SanitizedFields() uint64 }); ok {
		return sanitized.SanitizedFields()
	}
	return 0
}

func (w *fanoutTrapWriter) incOTLPExportFailed(n uint64) {
	if w.metrics != nil {
		w.metrics.addError("otlp_export_failed", n)
	}
}
