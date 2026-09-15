// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfile"

	"context"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type repository struct {
	User        string
	Name        string
	Status      int
	StarCount   int    `json:"star_count"`
	PullCount   int    `json:"pull_count"`
	LastUpdated string `json:"last_updated"`
}

func newAPIClient(client *http.Client, request web.RequestConfig, files credentialfile.RegularReader) *apiClient {
	return &apiClient{files: files, httpClient: client, request: request}
}

type apiClient struct {
	files      credentialfile.RegularReader
	httpClient *http.Client
	request    web.RequestConfig
}

func (a apiClient) getRepository(ctx context.Context, repoName string) (*repository, error) {
	req, err := a.createRequest(ctx, repoName)
	if err != nil {
		return nil, fmt.Errorf("error on creating http request : %v", err)
	}

	var repo repository
	if err := web.DoHTTP(a.httpClient).RequestJSON(req, &repo); err != nil {
		return nil, err
	}

	return &repo, nil
}

func (a apiClient) createRequest(ctx context.Context, urlPath string) (*http.Request, error) {
	req := a.request.Copy()

	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}

	u.Path = path.Join(u.Path, urlPath)
	req.URL = u.String()

	return web.NewHTTPRequest(ctx, req, a.files)
}
