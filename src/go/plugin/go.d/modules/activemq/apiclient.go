// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

type topics struct {
	XMLName xml.Name `xml:"topics"`
	Items   []topic  `xml:"topic"`
}

type topic struct {
	XMLName xml.Name `xml:"topic"`
	Name    string   `xml:"name,attr"`
	Stats   stats    `xml:"stats"`
}

type queues struct {
	XMLName xml.Name `xml:"queues"`
	Items   []queue  `xml:"queue"`
}

type queue struct {
	XMLName xml.Name `xml:"queue"`
	Name    string   `xml:"name,attr"`
	Stats   stats    `xml:"stats"`
}

type stats struct {
	XMLName       xml.Name `xml:"stats"`
	Size          int64    `xml:"size,attr"`
	ConsumerCount int64    `xml:"consumerCount,attr"`
	EnqueueCount  int64    `xml:"enqueueCount,attr"`
	DequeueCount  int64    `xml:"dequeueCount,attr"`
}

const pathStats = "/%s/xml/%s.jsp"

func newAPIClient(client *http.Client, request web.RequestConfig, webadmin string) *apiClient {
	return &apiClient{
		httpClient: client,
		request:    request,
		webadmin:   webadmin,
	}
}

type apiClient struct {
	httpClient *http.Client
	request    web.RequestConfig
	webadmin   string
}

func (a *apiClient) getQueues() (*queues, error) {
	req, err := a.createRequest(fmt.Sprintf(pathStats, a.webadmin, keyQueues))
	if err != nil {
		return nil, fmt.Errorf("error on creating request '%s' : %v", a.request.URL, err)
	}

	resp, err := a.doRequestOK(req)

	defer web.CloseBody(resp)

	if err != nil {
		return nil, err
	}

	var queues queues

	if err := xml.NewDecoder(resp.Body).Decode(&queues); err != nil {
		return nil, fmt.Errorf("error on decoding resp from %s : %s", req.URL, err)
	}

	return &queues, nil
}

func (a *apiClient) getTopics() (*topics, error) {
	req, err := a.createRequest(fmt.Sprintf(pathStats, a.webadmin, keyTopics))
	if err != nil {
		return nil, fmt.Errorf("error on creating request '%s' : %v", a.request.URL, err)
	}

	resp, err := a.doRequestOK(req)

	defer web.CloseBody(resp)

	if err != nil {
		return nil, err
	}

	var topics topics

	if err := xml.NewDecoder(resp.Body).Decode(&topics); err != nil {
		return nil, fmt.Errorf("error on decoding resp from %s : %s", req.URL, err)
	}

	return &topics, nil
}

func (a *apiClient) doRequestOK(req *http.Request) (*http.Response, error) {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return resp, fmt.Errorf("error on request to %s : %v", req.URL, err)
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s returned HTTPConfig status %d", req.URL, resp.StatusCode)
	}

	return resp, err
}

func (a *apiClient) createRequest(urlPath string) (*http.Request, error) {
	req := a.request.Copy()
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, urlPath)
	req.URL = u.String()
	return web.NewHTTPRequest(req)
}
