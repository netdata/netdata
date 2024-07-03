// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReader_Read(t *testing.T) {
	reader, teardown := prepareTestReader(t)
	defer teardown()

	r := testReader{bufio.NewReader(reader)}
	filename := reader.CurrentFilename()
	numLogs := 5
	var sum int

	for i := 0; i < 10; i++ {
		appendLogs(t, filename, time.Millisecond*10, numLogs)
		n, err := r.readUntilEOF()
		sum += n

		assert.Equal(t, io.EOF, err)
		assert.Equal(t, numLogs*(i+1), sum)
	}
}

func TestReader_Read_HandleFileRotation(t *testing.T) {
	reader, teardown := prepareTestReader(t)
	defer teardown()

	r := testReader{bufio.NewReader(reader)}
	filename := reader.CurrentFilename()
	numLogs := 5
	rotateFile(t, filename)
	appendLogs(t, filename, time.Millisecond*10, numLogs)

	n, err := r.readUntilEOFTimes(maxEOF)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, 0, n)

	appendLogs(t, filename, time.Millisecond*10, numLogs)
	n, err = r.readUntilEOF()
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, numLogs, n)
}

func TestReader_Read_HandleFileRotationWithDelay(t *testing.T) {
	reader, teardown := prepareTestReader(t)
	defer teardown()

	r := testReader{bufio.NewReader(reader)}
	filename := reader.CurrentFilename()
	_ = os.Remove(filename)

	// trigger reopen first time
	n, err := r.readUntilEOFTimes(maxEOF)
	assert.Equal(t, ErrNoMatchedFile, err)
	assert.Equal(t, 0, n)

	f, err := os.Create(filename)
	require.NoError(t, err)
	_ = f.Close()

	// trigger reopen 2nd time
	n, err = r.readUntilEOF()
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, 0, n)

	numLogs := 5
	appendLogs(t, filename, time.Millisecond*10, numLogs)
	n, err = r.readUntilEOF()
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, numLogs, n)
}

func TestReader_Close(t *testing.T) {
	reader, teardown := prepareTestReader(t)
	defer teardown()

	assert.NoError(t, reader.Close())
	assert.Nil(t, reader.file)
}

func TestReader_Close_NilFile(t *testing.T) {
	var r Reader
	assert.NoError(t, r.Close())
}

func TestOpen(t *testing.T) {
	tempFileName1 := prepareTempFile(t, "*-web_log-open-test-1.log")
	tempFileName2 := prepareTempFile(t, "*-web_log-open-test-2.log")
	tempFileName3 := prepareTempFile(t, "*-web_log-open-test-3.log")
	defer func() {
		_ = os.Remove(tempFileName1)
		_ = os.Remove(tempFileName2)
		_ = os.Remove(tempFileName3)
	}()

	makePath := func(s string) string {
		return filepath.Join(os.TempDir(), s)
	}

	tests := []struct {
		name    string
		path    string
		exclude string
		err     bool
	}{
		{
			name: "match without exclude",
			path: makePath("*-web_log-open-test-[1-3].log"),
		},
		{
			name:    "match with exclude",
			path:    makePath("*-web_log-open-test-[1-3].log"),
			exclude: makePath("*-web_log-open-test-[2-3].log"),
		},
		{
			name:    "exclude everything",
			path:    makePath("*-web_log-open-test-[1-3].log"),
			exclude: makePath("*"),
			err:     true,
		},
		{
			name: "no match",
			path: makePath("*-web_log-no-match-test-[1-3].log"),
			err:  true,
		},
		{
			name: "bad path pattern",
			path: "[qw",
			err:  true,
		},
		{
			name: "bad exclude path pattern",
			path: "[qw",
			err:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Open(tt.path, tt.exclude, nil)

			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, r.file)
				_ = r.Close()
			}
		})
	}
}

func TestReader_CurrentFilename(t *testing.T) {
	reader, teardown := prepareTestReader(t)
	defer teardown()

	assert.Equal(t, reader.file.Name(), reader.CurrentFilename())
}

type testReader struct {
	*bufio.Reader
}

func (r *testReader) readUntilEOF() (n int, err error) {
	for {
		_, err = r.ReadBytes('\n')
		if err != nil {
			break
		}
		n++
	}
	return n, err
}

func (r *testReader) readUntilEOFTimes(times int) (sum int, err error) {
	var n int
	for i := 0; i < times; i++ {
		n, err = r.readUntilEOF()
		if err != io.EOF {
			break
		}
		sum += n
	}
	return sum, err
}

func prepareTempFile(t *testing.T, pattern string) string {
	t.Helper()
	f, err := os.CreateTemp("", pattern)
	require.NoError(t, err)
	return f.Name()
}

func prepareTestReader(t *testing.T) (reader *Reader, teardown func()) {
	t.Helper()
	filename := prepareTempFile(t, "*-web_log-test.log")
	f, err := os.Open(filename)
	require.NoError(t, err)

	teardown = func() {
		_ = os.Remove(filename)
		_ = reader.file.Close()
	}
	reader = &Reader{
		file: f,
		path: filename,
	}
	return reader, teardown
}

func rotateFile(t *testing.T, filename string) {
	t.Helper()
	require.NoError(t, os.Remove(filename))
	f, err := os.Create(filename)
	require.NoError(t, err)
	_ = f.Close()
}

func appendLogs(t *testing.T, filename string, interval time.Duration, numOfLogs int) {
	t.Helper()
	base := filepath.Base(filename)
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_APPEND, os.ModeAppend)
	require.NoError(t, err)
	require.NotNil(t, file)
	defer func() { _ = file.Close() }()

	for i := 0; i < numOfLogs; i++ {
		_, err = fmt.Fprintln(file, "line", i, "filename", base)
		require.NoError(t, err)
		time.Sleep(interval)
	}
}
