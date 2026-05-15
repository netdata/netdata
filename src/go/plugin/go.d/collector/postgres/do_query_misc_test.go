// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloseDBAndUnregisterConnConfig_EmptyConnStr(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectClose()

	called := false
	original := unregisterConnConfig
	unregisterConnConfig = func(string) {
		called = true
	}
	defer func() { unregisterConnConfig = original }()

	closeDBAndUnregisterConnConfig(db, "")

	assert.False(t, called)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCloseDBAndUnregisterConnConfig_NonEmptyConnStr(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectClose()

	calledWith := ""
	original := unregisterConnConfig
	unregisterConnConfig = func(connStr string) {
		calledWith = connStr
	}
	defer func() { unregisterConnConfig = original }()

	closeDBAndUnregisterConnConfig(db, "registered-conn")

	assert.Equal(t, "registered-conn", calledWith)
	assert.NoError(t, mock.ExpectationsWereMet())
}
