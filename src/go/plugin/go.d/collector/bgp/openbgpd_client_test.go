// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenBGPDClient_DetectsBgplgdLayout(t *testing.T) {
	var neighborsHits, ribHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/neighbors":
			neighborsHits++
			_, _ = w.Write(dataOpenBGPDNeighbors)
		case "/rib":
			ribHits++
			assert.Equal(t, "", r.URL.RawQuery)
			_, _ = w.Write(dataOpenBGPDRIB)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newOpenBGPDClient(Config{APIURL: srv.URL, Timeout: confopt.Duration(2 * time.Second)})
	neighbors, err := client.Neighbors()
	require.NoError(t, err)
	assert.Equal(t, dataOpenBGPDNeighbors, neighbors)

	rib, err := client.RIB()
	require.NoError(t, err)
	assert.Equal(t, dataOpenBGPDRIB, rib)
	assert.Equal(t, 1, neighborsHits)
	assert.Equal(t, 1, ribHits)
}

func TestOpenBGPDClient_RIBFilteredUsesBgplgdQueryString(t *testing.T) {
	var neighborsHits, ribHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/neighbors":
			neighborsHits++
			_, _ = w.Write(dataOpenBGPDNeighbors)
		case "/rib":
			ribHits++
			assert.Equal(t, "af=ipv4", r.URL.RawQuery)
			_, _ = w.Write(dataOpenBGPDRIB)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newOpenBGPDClient(Config{APIURL: srv.URL, Timeout: confopt.Duration(2 * time.Second)})
	rib, err := client.RIBFiltered("ipv4")
	require.NoError(t, err)
	assert.Equal(t, dataOpenBGPDRIB, rib)
	assert.Equal(t, 1, neighborsHits)
	assert.Equal(t, 1, ribHits)
}

func TestOpenBGPDClient_DetectsStateServerLayout(t *testing.T) {
	var fallbackHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/neighbors":
			http.NotFound(w, r)
		case "/v1/bgpd/show/neighbor":
			fallbackHits++
			_, _ = w.Write(dataOpenBGPDNeighbors)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newOpenBGPDClient(Config{APIURL: srv.URL, Timeout: confopt.Duration(2 * time.Second)})
	neighbors, err := client.Neighbors()
	require.NoError(t, err)
	assert.Equal(t, dataOpenBGPDNeighbors, neighbors)
	assert.Equal(t, 1, fallbackHits)
}

func TestOpenBGPDClient_RIBUsesStateServerLayout(t *testing.T) {
	var neighborsHits, ribHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/neighbors":
			http.NotFound(w, r)
		case "/v1/bgpd/show/neighbor":
			neighborsHits++
			_, _ = w.Write(dataOpenBGPDNeighbors)
		case "/v1/bgpd/show/rib/detail":
			ribHits++
			assert.Equal(t, "", r.URL.RawQuery)
			_, _ = w.Write(dataOpenBGPDRIB)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newOpenBGPDClient(Config{APIURL: srv.URL, Timeout: confopt.Duration(2 * time.Second)})
	neighbors, err := client.Neighbors()
	require.NoError(t, err)
	assert.Equal(t, dataOpenBGPDNeighbors, neighbors)

	rib, err := client.RIB()
	require.NoError(t, err)
	assert.Equal(t, dataOpenBGPDRIB, rib)
	assert.Equal(t, 1, neighborsHits)
	assert.Equal(t, 1, ribHits)
}

func TestOpenBGPDClient_FallsBackFromStaleBgplgdLayoutToStateServer(t *testing.T) {
	var bgplgdNeighborHits, stateServerNeighborHits int
	bgplgdAvailable := true

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/neighbors":
			if !bgplgdAvailable {
				http.NotFound(w, r)
				return
			}
			bgplgdNeighborHits++
			_, _ = w.Write(dataOpenBGPDNeighbors)
		case "/v1/bgpd/show/neighbor":
			stateServerNeighborHits++
			_, _ = w.Write(dataOpenBGPDNeighbors)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newOpenBGPDClient(Config{APIURL: srv.URL, Timeout: confopt.Duration(2 * time.Second)})
	neighbors, err := client.Neighbors()
	require.NoError(t, err)
	assert.Equal(t, dataOpenBGPDNeighbors, neighbors)
	assert.Equal(t, 1, bgplgdNeighborHits)
	assert.Equal(t, 0, stateServerNeighborHits)

	bgplgdAvailable = false

	neighbors, err = client.Neighbors()
	require.NoError(t, err)
	assert.Equal(t, dataOpenBGPDNeighbors, neighbors)
	assert.Equal(t, 1, bgplgdNeighborHits)
	assert.Equal(t, 1, stateServerNeighborHits)
}

func TestOpenBGPDClient_RIBFilteredFallsBackFromStaleBgplgdLayoutToStateServer(t *testing.T) {
	var bgplgdNeighborHits, bgplgdRIBHits, stateServerNeighborHits int
	bgplgdAvailable := true

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/neighbors":
			if !bgplgdAvailable {
				http.NotFound(w, r)
				return
			}
			bgplgdNeighborHits++
			_, _ = w.Write(dataOpenBGPDNeighbors)
		case "/rib":
			if !bgplgdAvailable {
				http.NotFound(w, r)
				return
			}
			bgplgdRIBHits++
			assert.Equal(t, "af=ipv4", r.URL.RawQuery)
			_, _ = w.Write(dataOpenBGPDRIB)
		case "/v1/bgpd/show/neighbor":
			stateServerNeighborHits++
			_, _ = w.Write(dataOpenBGPDNeighbors)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newOpenBGPDClient(Config{APIURL: srv.URL, Timeout: confopt.Duration(2 * time.Second)})

	rib, err := client.RIBFiltered("ipv4")
	require.NoError(t, err)
	assert.Equal(t, dataOpenBGPDRIB, rib)
	assert.Equal(t, 1, bgplgdNeighborHits)
	assert.Equal(t, 1, bgplgdRIBHits)
	assert.Equal(t, 0, stateServerNeighborHits)

	bgplgdAvailable = false

	_, err = client.RIBFiltered("ipv4")
	require.Error(t, err)
	assert.ErrorIs(t, err, errOpenBGPDFilteredRIBUnsupported)
	assert.Equal(t, 1, bgplgdNeighborHits)
	assert.Equal(t, 1, bgplgdRIBHits)
	assert.Equal(t, 1, stateServerNeighborHits)
}

func TestOpenBGPDClient_RIBFilteredRejectsStateServerLayout(t *testing.T) {
	var fallbackHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/neighbors":
			http.NotFound(w, r)
		case "/v1/bgpd/show/neighbor":
			fallbackHits++
			_, _ = w.Write(dataOpenBGPDNeighbors)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newOpenBGPDClient(Config{APIURL: srv.URL, Timeout: confopt.Duration(2 * time.Second)})
	_, err := client.RIBFiltered("ipv4")
	require.Error(t, err)
	assert.ErrorIs(t, err, errOpenBGPDFilteredRIBUnsupported)
	assert.Equal(t, 1, fallbackHits)
}

func TestOpenBGPDClient_RIBFilteredRejectsUnsupportedFamilyBeforeHTTP(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := newOpenBGPDClient(Config{APIURL: srv.URL, Timeout: confopt.Duration(2 * time.Second)})
	_, err := client.RIBFiltered("evpn")
	require.Error(t, err)
	assert.ErrorIs(t, err, errOpenBGPDFilteredRIBFamilyUnsupported)
	assert.Equal(t, 0, hits)
}

func TestOpenBGPDClient_MapsPermissionErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := newOpenBGPDClient(Config{APIURL: srv.URL, Timeout: confopt.Duration(2 * time.Second)})
	_, err := client.Neighbors()
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrPermission)
}
