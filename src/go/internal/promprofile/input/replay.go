// SPDX-License-Identifier: GPL-3.0-or-later

package prominput

// MetadataExample identifies one deployable Prometheus metadata job.
type MetadataExample struct {
	IntegrationID string `yaml:"integration_id"`
	ExampleName   string `yaml:"example_name"`
	JobName       string `yaml:"job_name"`
}

// ReplayCase contains the resolved production inputs for one proof case.
type ReplayCase struct {
	ProfilePath            string
	SupportingProfilePaths []string
	FixturePaths           []string
	DefaultJobName         string
	MetadataPath           string
	MetadataExample        *MetadataExample
	FutureInputs           map[string]FutureInput
}
