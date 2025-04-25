package netdataexporter

//type ChartDefinition struct {
//	ID         string // must be globally unique
//	Title      string
//	Units      string
//	Family     string
//	Context    string // metric name
//	Type       string // line, area, stacked
//	Options    string // set to "obsolete" to delete the chart, otherwise empty string
//	Labels     []LabelDefinition
//	Dimensions []DimensionDefinition
//	IsNew      bool // Indicates if this chart is new and needs to be sent to Netdata
//}
//
//// LabelDefinition represents a Netdata chart label
//type LabelDefinition struct {
//	Name  string
//	Value string
//}
//
//// DimensionDefinition represents a Netdata chart dimension
//type DimensionDefinition struct {
//	ID    string // must be unique within the chart
//	Name  string // replaces ID in UI
//	Algo  string // absolute (Gauge), incremental (Counter)
//	Value float64
//}
