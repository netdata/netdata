// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testMntrData, _               = os.ReadFile("testdata/mntr.txt")
	testMntrNotInWhiteListData, _ = os.ReadFile("testdata/mntr_notinwhitelist.txt")
)

func Test_testDataLoad(t *testing.T) {
	assert.NotNil(t, testMntrData)
	assert.NotNil(t, testMntrNotInWhiteListData)
}

func TestNew(t *testing.T) {
	job := New()

	assert.IsType(t, (*Zookeeper)(nil), job)
}

func TestZookeeper_Init(t *testing.T) {
	job := New()

	assert.True(t, job.Init())
	assert.NotNil(t, job.fetcher)
}

func TestZookeeper_InitErrorOnCreatingTLSConfig(t *testing.T) {
	job := New()
	job.UseTLS = true
	job.TLSConfig.TLSCA = "testdata/tls"

	assert.False(t, job.Init())
}

func TestZookeeper_Check(t *testing.T) {
	job := New()
	require.True(t, job.Init())
	job.fetcher = &mockZookeeperFetcher{data: testMntrData}

	assert.True(t, job.Check())
}

func TestZookeeper_CheckErrorOnFetch(t *testing.T) {
	job := New()
	require.True(t, job.Init())
	job.fetcher = &mockZookeeperFetcher{err: true}

	assert.False(t, job.Check())
}

func TestZookeeper_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestZookeeper_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestZookeeper_Collect(t *testing.T) {
	job := New()
	require.True(t, job.Init())
	job.fetcher = &mockZookeeperFetcher{data: testMntrData}

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

	collected := job.Collect()

	assert.Equal(t, expected, collected)
	ensureCollectedHasAllChartsDimsVarsIDs(t, job, collected)
}

func TestZookeeper_CollectMntrNotInWhiteList(t *testing.T) {
	job := New()
	require.True(t, job.Init())
	job.fetcher = &mockZookeeperFetcher{data: testMntrNotInWhiteListData}

	assert.Nil(t, job.Collect())
}

func TestZookeeper_CollectMntrEmptyResponse(t *testing.T) {
	job := New()
	require.True(t, job.Init())
	job.fetcher = &mockZookeeperFetcher{}

	assert.Nil(t, job.Collect())
}

func TestZookeeper_CollectMntrInvalidData(t *testing.T) {
	job := New()
	require.True(t, job.Init())
	job.fetcher = &mockZookeeperFetcher{data: []byte("hello \nand good buy\n")}

	assert.Nil(t, job.Collect())
}

func TestZookeeper_CollectMntrReceiveError(t *testing.T) {
	job := New()
	require.True(t, job.Init())
	job.fetcher = &mockZookeeperFetcher{err: true}

	assert.Nil(t, job.Collect())
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, zk *Zookeeper, collected map[string]int64) {
	for _, chart := range *zk.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := collected[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := collected[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
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
