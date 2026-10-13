// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

// Capture names only a candidate file under this run's output directory.
// The Go artifact owner validates its location and computes bytes/digest.
type Capture struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path"`
	MIME string `json:"mime"`
}

type Artifact struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	MIME   string `json:"mime"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
