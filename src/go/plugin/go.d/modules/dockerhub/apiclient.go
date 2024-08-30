// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

type repository struct {
	User        string
	Name        string
	Status      int
	StarCount   int    `json:"star_count"`
	PullCount   int    `json:"pull_count"`
	LastUpdated string `json:"last_updated"`
}

func newAPIClient(client *http.Client, request web.Request) *apiClient {
	return &apiClient{httpClient: client, request: request}
}

type apiClient struct {
	httpClient *http.Client
	request    web.Request
}

func (a apiClient) getRepository(repoName string) (*repository, error) {
	req, err := a.createRequest(repoName)
	if err != nil {
		return nil, fmt.Errorf("error on creating http request : %v", err)
	}

	resp, err := a.doRequestOK(req)
	defer closeBody(resp)
	if err != nil {
		return nil, err
	}

	var repo repository
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return nil, fmt.Errorf("error on parsing response from %s : %v", req.URL, err)
	}

	return &repo, nil
}

func (a apiClient) doRequestOK(req *http.Request) (*http.Response, error) {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error on request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s returned HTTP status %d", req.URL, resp.StatusCode)
	}
	return resp, nil
}

func (a apiClient) createRequest(urlPath string) (*http.Request, error) {
	req := a.request.Copy()
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}

	u.Path = path.Join(u.Path, urlPath)
	req.URL = u.String()
	return web.NewHTTPRequest(req)
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
