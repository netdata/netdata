// SPDX-License-Identifier: GPL-3.0-or-later

package multipath

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	assert.Len(
		t,
		New("path1", "path2", "path2", "", "path3"),
		3,
	)
}

func TestMultiPath_Find(t *testing.T) {
	m := New("path1", "testdata/data1")

	v, err := m.Find("not exist")
	assert.Zero(t, v)
	assert.Error(t, err)

	v, err = m.Find("test-empty.conf")
	assert.Equal(t, "testdata/data1/test-empty.conf", v)
	assert.Nil(t, err)

	v, err = m.Find("test.conf")
	assert.Equal(t, "testdata/data1/test.conf", v)
	assert.Nil(t, err)
}

func TestIsNotFound(t *testing.T) {
	assert.True(t, IsNotFound(ErrNotFound{}))
	assert.False(t, IsNotFound(errors.New("")))
}

func TestMultiPath_FindFiles(t *testing.T) {
	m := New("path1", "testdata/data2", "testdata/data1")

	files, err := m.FindFiles(".conf")
	assert.NoError(t, err)
	assert.Equal(t, []string{"testdata/data2/test-empty.conf", "testdata/data2/test.conf"}, files)

	files, err = m.FindFiles()
	assert.NoError(t, err)
	assert.Equal(t, []string{"testdata/data2/test-empty.conf", "testdata/data2/test.conf"}, files)

	files, err = m.FindFiles(".not_exist")
	assert.NoError(t, err)
	assert.Equal(t, []string(nil), files)

	m = New("path1", "testdata/data1", "testdata/data2")
	files, err = m.FindFiles(".conf")
	assert.NoError(t, err)
	assert.Equal(t, []string{"testdata/data1/test-empty.conf", "testdata/data1/test.conf"}, files)
}
