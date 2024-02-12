// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	"errors"
	"net/http"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

func (cdb *CouchDB) validateConfig() error {
	if cdb.URL == "" {
		return errors.New("URL not set")
	}
	if cdb.Node == "" {
		return errors.New("'node' not set")
	}
	if _, err := web.NewHTTPRequest(cdb.Request); err != nil {
		return err
	}
	return nil
}

func (cdb *CouchDB) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(cdb.Client)
}

func (cdb *CouchDB) initCharts() (*Charts, error) {
	charts := module.Charts{}

	if err := charts.Add(*dbActivityCharts.Copy()...); err != nil {
		return nil, err
	}
	if err := charts.Add(*httpTrafficBreakdownCharts.Copy()...); err != nil {
		return nil, err
	}
	if err := charts.Add(*serverOperationsCharts.Copy()...); err != nil {
		return nil, err
	}
	if len(cdb.databases) != 0 {
		dbCharts := dbSpecificCharts.Copy()

		if err := charts.Add(*dbCharts...); err != nil {
			return nil, err
		}

		for _, chart := range *dbCharts {
			for _, db := range cdb.databases {
				if err := chart.AddDim(&module.Dim{ID: "db_" + db + "_" + chart.ID, Name: db}); err != nil {
					return nil, err
				}
			}
		}

	}
	if err := charts.Add(*erlangStatisticsCharts.Copy()...); err != nil {
		return nil, err
	}

	if len(charts) == 0 {
		return nil, errors.New("zero charts")
	}
	return &charts, nil
}
