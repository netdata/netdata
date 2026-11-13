// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import "encoding/json"

// Capture names only a candidate file under this run's output directory.
// The Go artifact owner validates its location and computes bytes/digest.
type Capture struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path"`
	MIME string `json:"mime"`
	// Test is joined by Go after core validation; it is never a wire authority.
	Test ArtifactTest `json:"-"`
}

type Artifact struct {
	ID     string       `json:"id"`
	Kind   string       `json:"kind"`
	MIME   string       `json:"mime"`
	Bytes  int64        `json:"bytes"`
	SHA256 string       `json:"sha256"`
	Test   ArtifactTest `json:"test,omitzero"`
}

type ArtifactTest struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	TextTruncated bool   `json:"text_truncated,omitempty"`
}

// Optional labels cannot make otherwise valid retained membership unreadable.
// Authoritative fields retain ordinary JSON type validation.
func (a *Artifact) UnmarshalJSON(raw []byte) error {
	type fields Artifact
	var value fields
	wire := struct {
		*fields
		Test json.RawMessage `json:"test"`
	}{fields: &value}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	if len(wire.Test) > 0 && json.Unmarshal(wire.Test, &value.Test) != nil {
		value.Test = ArtifactTest{}
	}
	*a = Artifact(value)
	return nil
}
