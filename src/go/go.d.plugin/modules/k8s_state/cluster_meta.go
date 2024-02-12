// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"fmt"
	"io"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (ks *KubeState) getKubeClusterID() string {
	ns, err := ks.client.CoreV1().Namespaces().Get(ks.ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		ks.Warningf("error on getting 'kube-system' namespace UID: %v", err)
		return ""
	}
	return string(ns.UID)
}

func (ks *KubeState) getKubeClusterName() string {
	client := http.Client{Timeout: time.Second}
	n, err := getGKEKubeClusterName(client)
	if err != nil {
		ks.Debugf("error on getting GKE cluster name: %v", err)
	}
	return n
}

func getGKEKubeClusterName(client http.Client) (string, error) {
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

func doMetaGKEHTTPReq(client http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer closeHTTPRespBody(resp)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("'%s' returned HTTP status code %d", url, resp.StatusCode)
	}

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	s := string(bs)
	if s == "" {
		return "", fmt.Errorf("an empty response from '%s'", url)
	}

	return s, nil
}

func closeHTTPRespBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
