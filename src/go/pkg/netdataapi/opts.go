// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

// ChartOpts contains all options needed to create a chart
type ChartOpts struct {
	TypeID      string
	ID          string
	Name        string
	Title       string
	Units       string
	Family      string
	Context     string
	ChartType   string
	Priority    int
	UpdateEvery int
	Options     string
	Plugin      string
	Module      string
}

// DimensionOpts contains all options needed to create a dimension
type DimensionOpts struct {
	ID         string
	Name       string
	Algorithm  string
	Multiplier int
	Divisor    int
	Options    string
}

// HostInfo contains the information needed for host definition
type HostInfo struct {
	GUID     string
	Hostname string
	Labels   map[string]string
}

// FunctionResult contains all parameters for a function result
type FunctionResult struct {
	UID             string
	ContentType     string
	Payload         string
	Code            string
	ExpireTimestamp string
}

// ConfigOpts contains options needed for config operations
type ConfigOpts struct {
	ID                string
	Status            string
	ConfigType        string
	Path              string
	SourceType        string
	Source            string
	SupportedCommands string
}
