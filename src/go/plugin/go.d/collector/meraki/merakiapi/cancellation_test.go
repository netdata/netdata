// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
	"github.com/stretchr/testify/require"
)

type cancelAtEOF struct {
	io.Reader
	cancel context.CancelFunc
}

func (r cancelAtEOF) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.cancel()
	}
	return n, err
}

func TestOperationsRejectCancellationAfterBufferedResponse(t *testing.T) {
	for _, operation := range []string{"list", "probe"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			client, err := merakiapi.New(merakiapi.Config{APIKey: syntheticKey}, &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					resp := response(req, http.StatusOK, nil, "")
					resp.Body = io.NopCloser(cancelAtEOF{Reader: strings.NewReader(`[]`), cancel: cancel})
					return resp, nil
				}),
			})
			require.NoError(t, err)
			if operation == "probe" {
				require.ErrorIs(t, client.ProbeOrganization(ctx, "org"), context.Canceled)
			} else {
				items, err := client.ListOrganizations(ctx)
				require.ErrorIs(t, err, context.Canceled)
				require.Nil(t, items)
			}
		})
	}
}
