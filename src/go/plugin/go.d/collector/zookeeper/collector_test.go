// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataMntrMetrics, _                = os.ReadFile("testdata/mntr.txt")
	dataMntrNotInWhiteListResponse, _ = os.ReadFile("testdata/mntr_notinwhitelist.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":                 dataConfigJSON,
		"dataConfigYAML":                 dataConfigYAML,
		"dataMntrMetrics":                dataMntrMetrics,
		"dataMntrNotInWhiteListResponse": dataMntrNotInWhiteListResponse,
	} {
		assert.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	collr := New()

	assert.NoError(t, collr.Init())
	assert.NotNil(t, collr.fetcher)
}

func TestCollector_InitErrorOnCreatingTLSConfig(t *testing.T) {
	collr := New()
	collr.UseTLS = true
	collr.TLSConfig.TLSCA = "testdata/tls"

	assert.Error(t, collr.Init())
}

func TestCollector_Check(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init())
	collr.fetcher = &mockZookeeperFetcher{data: dataMntrMetrics}

	assert.NoError(t, collr.Check())
}

func TestCollector_CheckErrorOnFetch(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init())
	collr.fetcher = &mockZookeeperFetcher{err: true}

	assert.Error(t, collr.Check())
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestCollector_Collect(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init())
	collr.fetcher = &mockZookeeperFetcher{data: dataMntrMetrics}

	expected := map[string]int64{
		"approximate_data_size":      44,
		"avg_latency":                100,
		"ephemerals_count":           0,
		"max_file_descriptor_count":  1048576,
		"max_latency":                100,
		"min_latency":                100,
		"num_alive_connections":      1,
		"open_file_descriptor_count": 63,
		"outstanding_requests":       0,
		"packets_received":           92,
		"packets_sent":               182,
		"server_state":               4,
		"watch_count":                0,
		"znode_count":                5,
	}

	mx := collr.Collect()

	assert.Equal(t, expected, mx)
	module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_CollectMntrNotInWhiteList(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init())
	collr.fetcher = &mockZookeeperFetcher{data: dataMntrNotInWhiteListResponse}

	assert.Nil(t, collr.Collect())
}

func TestCollector_CollectMntrEmptyResponse(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init())
	collr.fetcher = &mockZookeeperFetcher{}

	assert.Nil(t, collr.Collect())
}

func TestCollector_CollectMntrInvalidData(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init())
	collr.fetcher = &mockZookeeperFetcher{data: []byte("hello \nand good buy\n")}

	assert.Nil(t, collr.Collect())
}

func TestCollector_CollectMntrReceiveError(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init())
	collr.fetcher = &mockZookeeperFetcher{err: true}

	assert.Nil(t, collr.Collect())
}

type mockZookeeperFetcher struct {
	data []byte
	err  bool
}

func (m mockZookeeperFetcher) fetch(_ string) ([]string, error) {
	if m.err {
		return nil, errors.New("mock fetch error")
	}

	var lines []string
	s := bufio.NewScanner(bytes.NewReader(m.data))
	for s.Scan() {
		if !isZKLine(s.Bytes()) || isMntrLineOK(s.Bytes()) {
			lines = append(lines, s.Text())
		}
	}
	return lines, nil
}
