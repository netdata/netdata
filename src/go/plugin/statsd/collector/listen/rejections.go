// SPDX-License-Identifier: GPL-3.0-or-later

package listen

// rejection values are bounded diagnostic reasons, never input text.
type rejection string

func (r rejection) Error() string { return string(r) }

// Record rejections, from parsing, final preparation and admission.
const (
	rejectSyntax      rejection = "syntax"
	rejectValue       rejection = "value"
	rejectRate        rejection = "rate"
	rejectLabels      rejection = "labels"
	rejectMetadata    rejection = "metadata"
	rejectType        rejection = "type_conflict"
	rejectBaseline    rejection = "gauge_baseline"
	rejectCapacity    rejection = "capacity"
	rejectOverflow    rejection = "overflow"
	rejectUnavailable rejection = "receiver_unavailable"

	// Framing rejections, counted before record parsing.
	rejectOversize     rejection = "oversize"
	rejectUnterminated rejection = "unterminated"
)

// rejectReasons is the published rejection vocabulary. Input refused while the
// receiver is unavailable is not published: a stopped receiver has no output.
var rejectReasons = [...]rejection{
	rejectSyntax, rejectValue, rejectRate, rejectLabels, rejectMetadata, rejectType, rejectBaseline,
	rejectCapacity, rejectOverflow, rejectOversize, rejectUnterminated,
}
