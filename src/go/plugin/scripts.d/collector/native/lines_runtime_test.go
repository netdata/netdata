// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func linesCollector(t *testing.T, mode, body string) (*Collector, string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "collect.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/bash\nset -eu\n"+body), 0755))
	c := New()
	c.validateExecutable = statExecutable
	c.Command = []string{script}
	c.Mode = confopt.Enum[jobModeSpec](mode)
	c.SnapshotFormat = formatLines
	c.Timeout = confopt.Duration(time.Second)
	require.NoError(t, c.Init(context.Background()))
	return c, dir
}

func TestLines_OneshotAtomicityAndContracts(t *testing.T) {
	setupRunner(t)
	c, dir := linesCollector(t, modeOneshot, `cat "$(dirname "$0")/response"`+"\n")
	file := filepath.Join(dir, "response")
	write := func(data string) { require.NoError(t, os.WriteFile(file, []byte(data), 0644)) }
	write("depth:20|gauge|unit:jobs\ninvalid")
	require.Error(t, collectAndCommit(t, c))
	assert.Empty(t, rawSeries(c), "malformed output stages no partial observation")
	write("depth:17.5|gauge|unit:jobs")
	values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(17.5), values["depth"])
	for _, data := range []string{
		"depth:20|gauge|unit:jobs\ninvalid", // partial output must stage nothing
		"depth:20|counter|unit:jobs",        // retained type is shared with JSON
		"depth:20|gauge",                    // metadata does not inherit
	} {
		write(data)
		require.Error(t, collectAndCommit(t, c))
		assert.Equal(
			t,
			map[string]float64{"depth": 17.5},
			rawSeries(c),
			"failed observations never replace retained data",
		)
	}
	write("# no entities\n")
	values, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, values)
}

func TestLines_PersistentTransientFailure(t *testing.T) {
	setupRunner(t)
	c, _ := linesCollector(t, modePersistent, readyLine+`
read -r request
printf '%s\r\n' 'depth:1|gauge' '# EOF 1'
read -r request
printf '%s\n' '{"id":"2","error":"collection_failed"}'
read -r request
printf '%s\n' 'depth:3|gauge' '# EOF 3'
read -r request
`)
	startRuntime(t, c).waitReady(t)
	first, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(1), first["depth"])
	_, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.ErrorIs(t, err, errCollectionFailed)
	third, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(3), third["depth"])
}

func TestLines_PersistentTerminalReplies(t *testing.T) {
	setupRunner(t)
	cases := map[string]string{
		"wrong id":           `printf '%s\n' 'depth:1|gauge' '# EOF 2'`,
		"missing terminator": `printf '%s\n' 'depth:1|gauge'`,
		"partial eof":        `printf '%s' 'depth:1|gauge'; exec 1>&-`,
		"invalid record":     `printf '%s\n' 'depth:1|gauge' 'SYNTHETIC_SECRET' '# EOF 1'`,
		"json result":        `printf '%s\n' '{"id":"1","result":{"version":"v1"}}'`,
		"unknown error":      `printf '%s\n' '{"id":"1","error":"SYNTHETIC_SECRET"}'`,
		"error after data":   `printf '%s\n' 'depth:1|gauge' '{"id":"1","error":"collection_failed"}' '# EOF 1'`,
		"empty block":        `printf '%s\n' '# EOF 1'`,
		"blank block":        `printf '\n# EOF 1\n'`,
		"padded terminator":  `printf '%s\n' '# empty' ' # EOF 1'`,
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			c, _ := linesCollector(t, modePersistent, readyLine+"read -r request\n"+reply+"\nsleep 30\n")
			r := startRuntime(t, c)
			r.waitReady(t)
			require.Error(t, collectAndCommit(t, c))
			assert.Empty(t, rawSeries(c))
			r.wait(t)
			require.Error(t, r.err)
			assert.NotContains(t, r.err.Error(), "SYNTHETIC_SECRET")
		})
	}
}

func TestLines_PersistentUnsolicited(t *testing.T) {
	setupRunner(t)
	c, _ := linesCollector(
		t,
		modePersistent,
		readyLine+"read -r request\nprintf '%s\\n' '# empty' '# EOF 1' 'depth:999|gauge'\nsleep 30\n",
	)
	r := startRuntime(t, c)
	r.waitReady(t)
	_ = collectAndCommit(t, c)
	r.wait(t)
	require.ErrorContains(t, r.err, "unsolicited")
	require.Error(t, c.Collect(context.Background()))
}

func TestLines_PersistentWireLimit(t *testing.T) {
	setupRunner(t)
	python := requireTool(t, "python3")
	for _, tc := range []struct {
		name  string
		extra int
		crlf  bool
	}{
		{name: "exact"}, {name: "terminator pushes over", extra: 1}, {name: "crlf pushes over", extra: 1, crlf: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, dir := linesCollector(t, modePersistent, fmt.Sprintf("exec %q \"$(dirname \"$0\")/peer.py\"\n", python))
			ending := `b"\n"`
			if tc.crlf {
				ending = `b"\r\n"`
			}
			peer := fmt.Sprintf(`import sys
print('{"version":"v1","ready":true}', flush=True)
sys.stdin.readline()
end=%s
term=b"# EOF 1"+end
for size in [%d, %d-len(term)]:
    sys.stdout.buffer.write(b"#"+b"x"*(size-1-len(end))+end)
sys.stdout.buffer.write(term)
sys.stdout.buffer.flush()
sys.stdin.readline()
`, ending, maxMessageBytes/2, maxMessageBytes/2+tc.extra)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
			c.Timeout = confopt.Duration(15 * time.Second)
			r := startRuntime(t, c)
			r.waitReady(t)
			err := collectAndCommit(t, c)
			if tc.extra == 0 {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errResponseTooLarge)
				r.wait(t)
				require.ErrorIs(t, r.err, errResponseTooLarge)
			}
		})
	}
}

func TestLines_PersistentCancellationDrainsBlock(t *testing.T) {
	setupRunner(t)
	c, dir := linesCollector(t, modePersistent, readyLine+`
read -r request
printf '%s\n' 'depth:1|gauge'
printf started > "$(dirname "$0")/started"
while [[ ! -f "$(dirname "$0")/release" ]]; do sleep 0.01; done
printf '# EOF 1\n'
read -r request
printf '%s\n' 'depth:2|gauge' '# EOF 2'
read -r request
`)
	c.Timeout = confopt.Duration(5 * time.Second)
	r := startRuntime(t, c)
	r.waitReady(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Collect(ctx) }()
	waitFile(t, filepath.Join(dir, "started"))
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("caller did not cancel")
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
	values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(2), values["depth"])
	r.cancel()
	r.wait(t)
	require.ErrorIs(t, r.err, context.Canceled)
}

func TestLines_ConfigurationAndFunctions(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			_, dir := functionFixture(t, mode, false)
			peer := strings.Replace(
				functionPeer,
				`        print(json.dumps({"id":req["id"],"result":reply(req)}), flush=True)`,
				`        if req["method"] == "collect":
            print("depth:"+str(count)+"|gauge|#queue:mail|unit:jobs")
            print("# EOF "+req["id"], flush=True)
        else:
            print(json.dumps({"id":req["id"],"result":reply(req)}), flush=True)`,
				1,
			)
			peer = strings.Replace(
				peer,
				`    print(json.dumps(reply({"method":"collect"})))`,
				`    print("depth:"+str(count)+"|gauge|#queue:mail|unit:jobs")`,
				1,
			)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
			appendFile(t, filepath.Join(dir, "manifest.yaml"), "snapshot_format: lines\n")
			c := initFunctionCollector(t, loadTestPackages(t, filepath.Join(dir, "packages.yaml")))
			c.Timeout = confopt.Duration(5 * time.Second)
			if mode == modePersistent {
				startRuntime(t, c).waitReady(t)
			}
			for range 2 {
				values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.Equal(t, float64(23), values[`depth{queue="mail"}`])
				result, err := c.executeFunction(context.Background(), funcapi.RawMethodRequest{
					Method: "items",
				})
				require.NoError(t, err)
				assert.Equal(t, json.Number("23"), result.Data.([][]any)[0][0])
			}
		})
	}
}
