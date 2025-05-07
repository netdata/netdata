package netdataexporter

import (
	"io"
	"os"
	"strconv"
	"sync"
)

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

type DimensionOpts struct {
	ID         string
	Name       string
	Algorithm  string
	Multiplier int
	Divisor    int
	Options    string
}

type safeWriter struct {
	mx *sync.Mutex
	w  io.Writer
}

func (w *safeWriter) Write(p []byte) (n int, err error) {
	w.mx.Lock()
	n, err = w.w.Write(p)
	w.mx.Unlock()
	return n, err
}

func newNetdataStdoutApi() *netdataAPI {
	w := &safeWriter{
		mx: &sync.Mutex{},
		w:  os.Stdout,
	}
	return newNetdataApi(w)
}

func newNetdataApi(w io.Writer) *netdataAPI {
	if w == nil {
		panic("writer cannot be nil")
	}
	return &netdataAPI{w}
}

type netdataAPI struct {
	io.Writer
}

const quotes = "' '"

var (
	newLine = []byte("\n")
)

func (a *netdataAPI) chart(opts ChartOpts) {
	_, _ = a.Write([]byte("CHART " + "'" +
		opts.TypeID + "." + opts.ID + quotes +
		opts.Name + quotes +
		opts.Title + quotes +
		opts.Units + quotes +
		opts.Family + quotes +
		opts.Context + quotes +
		opts.ChartType + quotes +
		strconv.Itoa(opts.Priority) + quotes +
		strconv.Itoa(opts.UpdateEvery) + quotes +
		opts.Options + quotes +
		opts.Plugin + quotes +
		opts.Module + "'\n"))
}

func (a *netdataAPI) dimension(opts DimensionOpts) {
	_, _ = a.Write([]byte("DIMENSION '" +
		opts.ID + quotes +
		opts.Name + quotes +
		opts.Algorithm + quotes +
		strconv.Itoa(opts.Multiplier) + quotes +
		strconv.Itoa(opts.Divisor) + quotes +
		opts.Options + "'\n"))
}

func (a *netdataAPI) clabel(key, value string) {
	_, _ = a.Write([]byte("CLABEL '" +
		key + quotes +
		value + " '0'\n"))
}

// CLABELCOMMIT adds labels to the chart. Should be called after one or more CLABEL.
func (a *netdataAPI) clabelcommit() {
	_, _ = a.Write([]byte("CLABELCOMMIT\n"))
}

func (a *netdataAPI) begin(chartId string, msSince int) {
	if msSince > 0 {
		_, _ = a.Write([]byte("BEGIN " + "'" + chartId + "' " + strconv.Itoa(msSince) + "\n"))
	} else {
		_, _ = a.Write([]byte("BEGIN " + "'" + chartId + "'\n"))
	}
}

func (a *netdataAPI) set(dimensionId string, value int64) {
	_, _ = a.Write([]byte("SET '" + dimensionId + "' = " + strconv.FormatInt(value, 10) + "\n"))
}

// SETEMPTY sets an empty value for a dimension in the initialized chart.
func (a *netdataAPI) setempty(id string) {
	_, _ = a.Write([]byte("SET '" + id + "' = \n"))
}

func (a *netdataAPI) end() {
	_, _ = a.Write([]byte("END\n\n"))
}
