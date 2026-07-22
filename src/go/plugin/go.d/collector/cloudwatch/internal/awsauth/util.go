// SPDX-License-Identifier: GPL-3.0-or-later

package awsauth

// fieldPath joins a config path prefix with a field name for error messages
// (e.g. fieldPath("credentials.base", "type") -> "credentials.base.type").
func fieldPath(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}
