// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) getKubeClusterID() string {
	ns, err := c.client.CoreV1().Namespaces().Get(c.ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		c.Warningf("error on getting 'kube-system' namespace UID: %v", err)
		return ""
	}
	return string(ns.UID)
}

func (c *Collector) getKubeClusterName() string {
	client := &http.Client{Timeout: time.Second}
	n, err := getGKEKubeClusterName(client)
	if err != nil {
		c.Debugf("error on getting GKE cluster name: %v", err)
	}
	return n
}

func getGKEKubeClusterName(client *http.Client) (string, error) {
	id, err := doMetaGKEHTTPReq(client, "http://metadata/computeMetadata/v1/project/project-id")
	if err != nil {
		return "", err
	}
	loc, err := doMetaGKEHTTPReq(client, "http://metadata/computeMetadata/v1/instance/attributes/cluster-location")
	if err != nil {
		return "", err
	}
	name, err := doMetaGKEHTTPReq(client, "http://metadata/computeMetadata/v1/instance/attributes/cluster-name")
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("gke_%s_%s_%s", id, loc, name), nil
}

func doMetaGKEHTTPReq(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Metadata-Flavor", "Google")

	var resp string

	if err := web.DoHTTP(client).Request(req, func(body io.Reader) error {
		bs, rerr := io.ReadAll(body)
		if rerr != nil {
			return rerr
		}

		if resp = string(bs); len(resp) == 0 {
			return errors.New("empty response")
		}

		return nil
	}); err != nil {
		return "", err
	}

	return resp, nil
}
