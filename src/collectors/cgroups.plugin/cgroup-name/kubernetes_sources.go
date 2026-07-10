// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type podMetadataResult struct {
	kubeSystemNamespace string
	pods                string
	outcome             kubePodOutcome
}

type httpGetter func(context.Context, string, httpGetOptions) ([]byte, error)

func (r *resolver) k8sGCPGetClusterName(ctx context.Context) (string, bool) {
	return r.k8sGCPGetClusterNameWith(ctx, httpGetWithContext)
}

func (r *resolver) k8sGCPGetClusterNameWith(ctx context.Context, get httpGetter) (string, bool) {
	type result struct {
		index int
		value string
		err   error
	}
	baseURL := r.config.kubernetes.gcpMetadataURL
	if baseURL == "" {
		baseURL = defaultGCPMetadataURL
	}
	baseURL = strings.TrimRight(baseURL, "/") + "/computeMetadata/v1"
	urls := []string{
		baseURL + "/project/project-id",
		baseURL + "/instance/attributes/cluster-location",
		baseURL + "/instance/attributes/cluster-name",
	}
	headers := map[string]string{"Metadata-Flavor": "Google"}
	defer r.track("gcp-metadata", time.Now())
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan result, len(urls))
	var waitGroup sync.WaitGroup
	for index, url := range urls {
		waitGroup.Add(1)
		go func(index int, url string) {
			defer waitGroup.Done()
			body, err := get(ctx, url, httpGetOptions{
				headers: headers,
				noProxy: true,
				fail:    true,
				timeout: 3 * time.Second,
			})
			value := strings.TrimSpace(string(body))
			if err != nil || value == "" {
				cancel()
			}
			results <- result{index: index, value: value, err: err}
		}(index, url)
	}
	waitGroup.Wait()
	close(results)

	values := make([]string, len(urls))
	for result := range results {
		if result.err != nil || result.value == "" {
			return "", false
		}
		values[result.index] = result.value
	}
	return "gke_" + values[0] + "_" + values[1] + "_" + values[2], true
}

func (r *resolver) k8sFetchPods(ctx context.Context, functionName, kubeSystemUID string) podMetadataResult {
	config := r.config.kubernetes
	if config.serviceHost != "" && config.servicePort != "" {
		token, _ := os.ReadFile(config.serviceAccountTokenFile)
		headers := map[string]string{"Authorization": "Bearer " + strings.TrimSpace(string(token))}
		host := config.serviceHost + ":" + config.servicePort

		var kubeSystemNamespace string
		if kubeSystemUID == "" {
			url := "https://" + host + "/api/v1/namespaces/kube-system"
			started := time.Now()
			body, err := httpGetWithContext(ctx, url, httpGetOptions{
				headers:   headers,
				tlsConfig: r.k8sTLSConfig(tlsModeAPIServer),
				fail:      true,
			})
			r.track("k8s-api-namespace", started)
			if err != nil {
				r.warning(fmt.Sprintf("%s: error on curl '%s': %s.%s", functionName, url, err.Error(), tlsHint(err)))
			} else {
				kubeSystemNamespace = string(body)
			}
		}

		url := "https://" + host + "/api/v1/pods"
		tlsMode := tlsModeAPIServer
		if config.useKubelet {
			url = kubeletPodsURL(config.kubeletURL)
			tlsMode = tlsModeKubelet
		} else if config.nodeName != "" {
			url += "?fieldSelector=spec.nodeName==" + config.nodeName
		}

		started := time.Now()
		body, err := httpGetWithContext(ctx, url, httpGetOptions{
			headers:   headers,
			tlsConfig: r.k8sTLSConfig(tlsMode),
			fail:      true,
			maxBody:   k8sPodsBodyCap,
		})
		r.track("k8s-pods", started)
		if err != nil {
			r.warning(fmt.Sprintf("%s: error on curl '%s': %s.%s", functionName, url, err.Error(), tlsHint(err)))
			return podMetadataResult{kubeSystemNamespace: kubeSystemNamespace, outcome: kubePodEnableFallback}
		}
		return podMetadataResult{
			kubeSystemNamespace: kubeSystemNamespace,
			pods:                string(body),
			outcome:             kubePodSuccess,
		}
	}

	if r.kubeletProcessRunning(ctx) && commandAvailable("kubectl") {
		// Preserve the legacy unset-versus-empty quirk: the namespace query uses
		// kubectl defaults when KUBE_CONFIG is unset, while only the pods query
		// receives /etc/kubernetes/admin.conf.
		kubeConfig := config.kubeConfig
		var kubeSystemNamespace string
		if kubeSystemUID == "" {
			out, err := r.kubectlOutput(ctx, kubeConfig, defaultBodyCap, "get", "namespaces", "kube-system", "-o", "json")
			if err != nil {
				r.warning(fmt.Sprintf("%s: error on 'kubectl': %s.", functionName, strings.TrimRight(string(out), "\n")))
			} else {
				kubeSystemNamespace = string(out)
			}
		}
		if !config.kubeConfigSet {
			kubeConfig = "/etc/kubernetes/admin.conf"
		}
		out, err := r.kubectlOutput(ctx, kubeConfig, k8sPodsBodyCap, "get", "pods", "--all-namespaces", "-o", "json")
		if err != nil {
			r.warning(fmt.Sprintf("%s: error on 'kubectl': %s.", functionName, strings.TrimRight(string(out), "\n")))
			return podMetadataResult{kubeSystemNamespace: kubeSystemNamespace, outcome: kubePodEnableFallback}
		}
		return podMetadataResult{
			kubeSystemNamespace: kubeSystemNamespace,
			pods:                string(out),
			outcome:             kubePodSuccess,
		}
	}

	r.warning(fmt.Sprintf("%s: not inside the k8s cluster and 'kubectl' command not available.", functionName))
	return podMetadataResult{outcome: kubePodEnableFallback}
}

func (r *resolver) kubeletProcessRunning(ctx context.Context) bool {
	defer r.track("ps-kubelet", time.Now())
	return exec.CommandContext(ctx, "ps", "-C", "kubelet").Run() == nil
}

func (r *resolver) kubectlOutput(ctx context.Context, kubeConfig string, maxOutput int64, args ...string) ([]byte, error) {
	defer r.track("kubectl", time.Now())
	fullArgs := append([]string{"--kubeconfig=" + kubeConfig}, args...)
	return commandCombinedOutput(ctx, "kubectl", maxOutput, fullArgs...)
}

type cappedOutput struct {
	buffer   bytes.Buffer
	limit    int64
	exceeded bool
}

func (w *cappedOutput) Write(value []byte) (int, error) {
	length := len(value)
	remaining := w.limit - int64(w.buffer.Len())
	if remaining <= 0 {
		w.exceeded = true
		return length, nil
	}
	if int64(length) > remaining {
		_, _ = w.buffer.Write(value[:remaining])
		w.exceeded = true
		return length, nil
	}
	_, _ = w.buffer.Write(value)
	return length, nil
}

func commandCombinedOutput(ctx context.Context, command string, maxOutput int64, args ...string) ([]byte, error) {
	if maxOutput <= 0 {
		maxOutput = defaultBodyCap
	}
	output := cappedOutput{limit: maxOutput}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if output.exceeded {
		return output.buffer.Bytes(), fmt.Errorf("%s output exceeds %d bytes", command, maxOutput)
	}
	return output.buffer.Bytes(), err
}

func kubeletPodsURL(base string) string {
	return base + "/pods"
}
