// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import (
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/web"
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
	req, err := web.NewHTTPRequestWithPath(a.request, fmt.Sprintf(pathStats, a.webadmin, keyQueues))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request '%s': %v", a.request.URL, err)
	}

	var queues queues

	if err := web.DoHTTP(a.httpClient).RequestXML(req, &queues); err != nil {
		return nil, err
	}

	return &queues, nil
}

func (a *apiClient) getTopics() (*topics, error) {
	req, err := web.NewHTTPRequestWithPath(a.request, fmt.Sprintf(pathStats, a.webadmin, keyTopics))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request '%s': %v", a.request.URL, err)
	}

	var topics topics

	if err := web.DoHTTP(a.httpClient).RequestXML(req, &topics); err != nil {
		return nil, err
	}

	return &topics, nil
}
