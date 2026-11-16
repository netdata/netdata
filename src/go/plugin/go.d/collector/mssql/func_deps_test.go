// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mssql/mssqlfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMSSQLFunctions_SharedServerInfo(t *testing.T) {
	for name, tc := range map[string]struct {
		version    string
		edition    int
		catalog    string
		permission string
	}{
		"SQL Server 2017": {"14.0.3540.1", 3, "server", "VIEW SERVER STATE"},
		"SQL Server 2022": {"16.0.4265.3", 3, "server", "VIEW SERVER PERFORMANCE STATE"},
		"Azure Database":  {"12.0.2000.8", 5, "database", "VIEW DATABASE PERFORMANCE STATE"},
		"Azure MI":        {"16.0.4265.3", 8, "server", "VIEW SERVER PERFORMANCE STATE"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, loadedByMetrics := range []bool{false, true} {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()

				c := New()
				c.functionDB = db
				handler := mssqlfunc.NewRouter(functionDeps{
					collector: c,
				}, c.Logger, c.Functions)
				if loadedByMetrics {
					// The router must observe properties populated after its construction.
					c.setServerProperties(tc.version, tc.edition)
				} else {
					mock.ExpectQuery("ProductVersion.*EngineEdition").
						WillReturnRows(sqlmock.NewRows([]string{"version", "engine_edition"}).AddRow(tc.version, tc.edition))
				}

				for range 2 {
					mock.ExpectQuery(tc.catalog + "_event_session_fields").
						WillReturnError(errors.New("permission was denied"))
					response := handler.Handle(context.Background(), "error-info", nil)
					assert.Equal(t, 403, response.Status, response.Message)
					assert.Contains(t, response.Message, tc.permission)
				}
				version, _, edition, loaded := c.serverProperties()
				assert.True(t, loaded)
				assert.Equal(t, tc.version, version)
				assert.Equal(t, tc.edition, edition)
				assert.Nil(t, c.db, "Functions must not open a metrics pool")
				require.NoError(t, mock.ExpectationsWereMet())
			}
		})
	}
}
