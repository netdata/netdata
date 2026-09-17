// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import "time"

// Resource is the current acquisition's input to measurement conversion. Documents,
// excerpts and response timing must describe this cycle; retained identities carry
// no old readings. Project treats every field and referenced document as read-only.
type Resource struct {
	Kind             string
	URI              string
	Key              string
	Data             map[string]any
	Enrichment       map[string]Enrichment
	Doc              Document
	AcquisitionState string
	SourcePath       string
	SourceModel      string
	Response         ResponseTiming
	SensorExcerpts   []SensorExcerpt
}

// Enrichment preserves the fetched document's URI for relative reading provenance.
type Enrichment struct {
	Data map[string]any
	URI  string
}

type SensorExcerpt struct {
	Path  string
	Type  string
	Units string
	Data  map[string]any
}

type ResponseTiming struct {
	StartedAt  time.Time
	FinishedAt time.Time
}

type Link struct {
	ODataID string `json:"@odata.id"`
}
