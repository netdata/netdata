// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"errors"
	"io"

	"gopkg.in/yaml.v2"
)

var (
	errYAMLSchema    = errors.New("does not match the schema")
	errYAMLDocuments = errors.New("must contain one YAML document")
)

// decodeYAMLDocument strictly decodes exactly one YAML document. Errors omit
// decoder details, which can quote document values.
func decodeYAMLDocument(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.SetStrict(true)
	if decoder.Decode(target) != nil {
		return errYAMLSchema
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errYAMLDocuments
	}
	return nil
}
