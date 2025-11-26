// SPDX-License-Identifier: GPL-3.0-or-later

// Kubernetes Metrics Collector for Netdata Alternative UI
//
// This collector gathers metrics from Kubernetes clusters and pushes them
// to the alternative UI server for visualization.
//
// Features:
// - Pod CPU/Memory metrics
// - Node resource utilization
// - Deployment/ReplicaSet status
// - Container-level metrics
// - Kubernetes events
// - Namespace-aware metrics grouping
//
// Build:
//   go build -o k8s-collector .
//
// Usage:
//   ./k8s-collector --url http://localhost:19998 --kubeconfig ~/.kube/config

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// MetricPush is the payload sent to the server
type MetricPush struct {
	NodeID    string            `json:"node_id"`
	NodeName  string            `json:"node_name,omitempty"`
	Hostname  string            `json:"hostname,omitempty"`
	OS        string            `json:"os,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Charts    []Chart           `json:"charts"`
	Timestamp int64             `json:"timestamp"`
}

// Chart represents a metric chart
type Chart struct {
	ID          string      `json:"id"`
	Type        string      `json:"type,omitempty"`
	Name        string      `json:"name,omitempty"`
	Title       string      `json:"title,omitempty"`
	Units       string      `json:"units,omitempty"`
	Family      string      `json:"family,omitempty"`
	Context     string      `json:"context,omitempty"`
	ChartType   string      `json:"chart_type,omitempty"`
	Priority    int         `json:"priority,omitempty"`
	UpdateEvery int         `json:"update_every,omitempty"`
	Dimensions  []Dimension `json:"dimensions"`
}

// Dimension represents a metric dimension
type Dimension struct {
	ID         string  `json:"id"`
	Name       string  `json:"name,omitempty"`
	Algorithm  string  `json:"algorithm,omitempty"`
	Multiplier float64 `json:"multiplier,omitempty"`
	Divisor    float64 `json:"divisor,omitempty"`
	Value      float64 `json:"value"`
}

// K8sCollector collects Kubernetes metrics
type K8sCollector struct {
	clientset     *kubernetes.Clientset
	metricsClient *metricsclient.Clientset
	pushURL       string
	apiKey        string
	clusterName   string
	namespaces    []string
	httpClient    *http.Client
}

// NewK8sCollector creates a new Kubernetes collector
func NewK8sCollector(kubeconfig, pushURL, apiKey, clusterName string, namespaces []string) (*K8sCollector, error) {
	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		if kubeconfig == "" {
			home, _ := os.UserHomeDir()
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	metricsClient, err := metricsclient.NewForConfig(config)
	if err != nil {
		log.Printf("Warning: metrics-server not available: %v", err)
	}

	return &K8sCollector{
		clientset:     clientset,
		metricsClient: metricsClient,
		pushURL:       pushURL,
		apiKey:        apiKey,
		clusterName:   clusterName,
		namespaces:    namespaces,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Collect gathers all Kubernetes metrics
func (c *K8sCollector) Collect(ctx context.Context) error {
	charts := make([]Chart, 0)

	// Collect node metrics
	nodeCharts, err := c.collectNodeMetrics(ctx)
	if err != nil {
		log.Printf("Error collecting node metrics: %v", err)
	} else {
		charts = append(charts, nodeCharts...)
	}

	// Collect pod metrics per namespace
	namespaces := c.namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""} // All namespaces
	}

	for _, ns := range namespaces {
		podCharts, err := c.collectPodMetrics(ctx, ns)
		if err != nil {
			log.Printf("Error collecting pod metrics for namespace %s: %v", ns, err)
		} else {
			charts = append(charts, podCharts...)
		}

		deploymentCharts, err := c.collectDeploymentMetrics(ctx, ns)
		if err != nil {
			log.Printf("Error collecting deployment metrics: %v", err)
		} else {
			charts = append(charts, deploymentCharts...)
		}
	}

	// Collect cluster-wide metrics
	clusterCharts, err := c.collectClusterMetrics(ctx)
	if err != nil {
		log.Printf("Error collecting cluster metrics: %v", err)
	} else {
		charts = append(charts, clusterCharts...)
	}

	// Push metrics
	return c.push(charts)
}

// collectNodeMetrics collects metrics from Kubernetes nodes
func (c *K8sCollector) collectNodeMetrics(ctx context.Context) ([]Chart, error) {
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	charts := make([]Chart, 0)

	// Node status chart
	nodeDims := make([]Dimension, 0)
	for _, node := range nodes.Items {
		status := 0.0
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				status = 1.0
				break
			}
		}
		nodeDims = append(nodeDims, Dimension{
			ID:    sanitizeID(node.Name),
			Name:  node.Name,
			Value: status,
		})
	}

	charts = append(charts, Chart{
		ID:        "k8s.nodes.status",
		Type:      "k8s",
		Title:     "Node Status",
		Units:     "status",
		Family:    "nodes",
		Context:   "k8s.nodes.status",
		ChartType: "line",
		Priority:  100,
		Dimensions: nodeDims,
	})

	// Try to get node metrics from metrics-server
	if c.metricsClient != nil {
		nodeMetrics, err := c.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
		if err == nil {
			cpuDims := make([]Dimension, 0)
			memDims := make([]Dimension, 0)

			for _, nm := range nodeMetrics.Items {
				cpuDims = append(cpuDims, Dimension{
					ID:    sanitizeID(nm.Name) + "_cpu",
					Name:  nm.Name,
					Value: float64(nm.Usage.Cpu().MilliValue()),
				})
				memDims = append(memDims, Dimension{
					ID:    sanitizeID(nm.Name) + "_mem",
					Name:  nm.Name,
					Value: float64(nm.Usage.Memory().Value()) / (1024 * 1024), // MiB
				})
			}

			charts = append(charts, Chart{
				ID:         "k8s.nodes.cpu",
				Type:       "k8s",
				Title:      "Node CPU Usage",
				Units:      "millicores",
				Family:     "nodes",
				Context:    "k8s.nodes.cpu",
				ChartType:  "stacked",
				Priority:   110,
				Dimensions: cpuDims,
			})

			charts = append(charts, Chart{
				ID:         "k8s.nodes.memory",
				Type:       "k8s",
				Title:      "Node Memory Usage",
				Units:      "MiB",
				Family:     "nodes",
				Context:    "k8s.nodes.memory",
				ChartType:  "stacked",
				Priority:   120,
				Dimensions: memDims,
			})
		}
	}

	// Node capacity chart
	cpuCapDims := make([]Dimension, 0)
	memCapDims := make([]Dimension, 0)
	podCapDims := make([]Dimension, 0)

	for _, node := range nodes.Items {
		cpuCapDims = append(cpuCapDims, Dimension{
			ID:    sanitizeID(node.Name) + "_cpu_cap",
			Name:  node.Name,
			Value: float64(node.Status.Capacity.Cpu().MilliValue()),
		})
		memCapDims = append(memCapDims, Dimension{
			ID:    sanitizeID(node.Name) + "_mem_cap",
			Name:  node.Name,
			Value: float64(node.Status.Capacity.Memory().Value()) / (1024 * 1024 * 1024), // GiB
		})
		podCapDims = append(podCapDims, Dimension{
			ID:    sanitizeID(node.Name) + "_pod_cap",
			Name:  node.Name,
			Value: float64(node.Status.Capacity.Pods().Value()),
		})
	}

	charts = append(charts, Chart{
		ID:         "k8s.nodes.cpu_capacity",
		Type:       "k8s",
		Title:      "Node CPU Capacity",
		Units:      "millicores",
		Family:     "nodes",
		Context:    "k8s.nodes.cpu_capacity",
		ChartType:  "line",
		Priority:   130,
		Dimensions: cpuCapDims,
	})

	charts = append(charts, Chart{
		ID:         "k8s.nodes.memory_capacity",
		Type:       "k8s",
		Title:      "Node Memory Capacity",
		Units:      "GiB",
		Family:     "nodes",
		Context:    "k8s.nodes.memory_capacity",
		ChartType:  "line",
		Priority:   140,
		Dimensions: memCapDims,
	})

	return charts, nil
}

// collectPodMetrics collects metrics from Kubernetes pods
func (c *K8sCollector) collectPodMetrics(ctx context.Context, namespace string) ([]Chart, error) {
	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	charts := make([]Chart, 0)

	// Group pods by namespace
	podsByNs := make(map[string][]corev1.Pod)
	for _, pod := range pods.Items {
		podsByNs[pod.Namespace] = append(podsByNs[pod.Namespace], pod)
	}

	// Pod counts by phase per namespace
	for ns, nsPods := range podsByNs {
		phaseCounts := map[string]float64{
			"Running":   0,
			"Pending":   0,
			"Succeeded": 0,
			"Failed":    0,
			"Unknown":   0,
		}

		for _, pod := range nsPods {
			phase := string(pod.Status.Phase)
			phaseCounts[phase]++
		}

		dims := make([]Dimension, 0)
		for phase, count := range phaseCounts {
			dims = append(dims, Dimension{
				ID:    strings.ToLower(phase),
				Name:  phase,
				Value: count,
			})
		}

		// Sort dimensions for consistent ordering
		sort.Slice(dims, func(i, j int) bool {
			return dims[i].ID < dims[j].ID
		})

		charts = append(charts, Chart{
			ID:         fmt.Sprintf("k8s.pods.%s.status", sanitizeID(ns)),
			Type:       "k8s",
			Title:      fmt.Sprintf("Pod Status (%s)", ns),
			Units:      "pods",
			Family:     "pods",
			Context:    "k8s.pods.status",
			ChartType:  "stacked",
			Priority:   200,
			Dimensions: dims,
		})
	}

	// Try to get pod metrics from metrics-server
	if c.metricsClient != nil {
		podMetrics, err := c.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			// Group by namespace
			metricsByNs := make(map[string][]metricsv1beta1.PodMetrics)
			for _, pm := range podMetrics.Items {
				metricsByNs[pm.Namespace] = append(metricsByNs[pm.Namespace], pm)
			}

			for ns, nsMetrics := range metricsByNs {
				cpuDims := make([]Dimension, 0)
				memDims := make([]Dimension, 0)

				for _, pm := range nsMetrics {
					var cpuTotal int64
					var memTotal int64

					for _, container := range pm.Containers {
						cpuTotal += container.Usage.Cpu().MilliValue()
						memTotal += container.Usage.Memory().Value()
					}

					cpuDims = append(cpuDims, Dimension{
						ID:    sanitizeID(pm.Name),
						Name:  pm.Name,
						Value: float64(cpuTotal),
					})
					memDims = append(memDims, Dimension{
						ID:    sanitizeID(pm.Name),
						Name:  pm.Name,
						Value: float64(memTotal) / (1024 * 1024), // MiB
					})
				}

				if len(cpuDims) > 0 {
					// Limit to top 20 pods by CPU
					if len(cpuDims) > 20 {
						sort.Slice(cpuDims, func(i, j int) bool {
							return cpuDims[i].Value > cpuDims[j].Value
						})
						cpuDims = cpuDims[:20]
					}

					charts = append(charts, Chart{
						ID:         fmt.Sprintf("k8s.pods.%s.cpu", sanitizeID(ns)),
						Type:       "k8s",
						Title:      fmt.Sprintf("Pod CPU Usage (%s)", ns),
						Units:      "millicores",
						Family:     "pods",
						Context:    "k8s.pods.cpu",
						ChartType:  "stacked",
						Priority:   210,
						Dimensions: cpuDims,
					})
				}

				if len(memDims) > 0 {
					// Limit to top 20 pods by memory
					if len(memDims) > 20 {
						sort.Slice(memDims, func(i, j int) bool {
							return memDims[i].Value > memDims[j].Value
						})
						memDims = memDims[:20]
					}

					charts = append(charts, Chart{
						ID:         fmt.Sprintf("k8s.pods.%s.memory", sanitizeID(ns)),
						Type:       "k8s",
						Title:      fmt.Sprintf("Pod Memory Usage (%s)", ns),
						Units:      "MiB",
						Family:     "pods",
						Context:    "k8s.pods.memory",
						ChartType:  "stacked",
						Priority:   220,
						Dimensions: memDims,
					})
				}
			}
		}
	}

	// Container restart counts
	restartDims := make([]Dimension, 0)
	for _, pod := range pods.Items {
		var totalRestarts int32
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
		}
		if totalRestarts > 0 {
			restartDims = append(restartDims, Dimension{
				ID:    sanitizeID(pod.Namespace + "_" + pod.Name),
				Name:  fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Value: float64(totalRestarts),
			})
		}
	}

	if len(restartDims) > 0 {
		// Limit to top 20 by restarts
		if len(restartDims) > 20 {
			sort.Slice(restartDims, func(i, j int) bool {
				return restartDims[i].Value > restartDims[j].Value
			})
			restartDims = restartDims[:20]
		}

		charts = append(charts, Chart{
			ID:         "k8s.pods.restarts",
			Type:       "k8s",
			Title:      "Container Restarts",
			Units:      "restarts",
			Family:     "pods",
			Context:    "k8s.pods.restarts",
			ChartType:  "line",
			Priority:   230,
			Dimensions: restartDims,
		})
	}

	return charts, nil
}

// collectDeploymentMetrics collects metrics from Kubernetes deployments
func (c *K8sCollector) collectDeploymentMetrics(ctx context.Context, namespace string) ([]Chart, error) {
	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	charts := make([]Chart, 0)

	// Group by namespace
	depsByNs := make(map[string][]struct {
		name     string
		desired  int32
		ready    int32
		available int32
	})

	for _, dep := range deployments.Items {
		depsByNs[dep.Namespace] = append(depsByNs[dep.Namespace], struct {
			name      string
			desired   int32
			ready     int32
			available int32
		}{
			name:      dep.Name,
			desired:   *dep.Spec.Replicas,
			ready:     dep.Status.ReadyReplicas,
			available: dep.Status.AvailableReplicas,
		})
	}

	for ns, deps := range depsByNs {
		desiredDims := make([]Dimension, 0)
		readyDims := make([]Dimension, 0)
		availableDims := make([]Dimension, 0)

		for _, dep := range deps {
			desiredDims = append(desiredDims, Dimension{
				ID:    sanitizeID(dep.name) + "_desired",
				Name:  dep.name,
				Value: float64(dep.desired),
			})
			readyDims = append(readyDims, Dimension{
				ID:    sanitizeID(dep.name) + "_ready",
				Name:  dep.name,
				Value: float64(dep.ready),
			})
			availableDims = append(availableDims, Dimension{
				ID:    sanitizeID(dep.name) + "_available",
				Name:  dep.name,
				Value: float64(dep.available),
			})
		}

		if len(desiredDims) > 0 {
			charts = append(charts, Chart{
				ID:         fmt.Sprintf("k8s.deployments.%s.replicas", sanitizeID(ns)),
				Type:       "k8s",
				Title:      fmt.Sprintf("Deployment Replicas (%s)", ns),
				Units:      "replicas",
				Family:     "deployments",
				Context:    "k8s.deployments.replicas",
				ChartType:  "line",
				Priority:   300,
				Dimensions: desiredDims,
			})

			charts = append(charts, Chart{
				ID:         fmt.Sprintf("k8s.deployments.%s.ready", sanitizeID(ns)),
				Type:       "k8s",
				Title:      fmt.Sprintf("Deployment Ready (%s)", ns),
				Units:      "replicas",
				Family:     "deployments",
				Context:    "k8s.deployments.ready",
				ChartType:  "line",
				Priority:   310,
				Dimensions: readyDims,
			})
		}
	}

	return charts, nil
}

// collectClusterMetrics collects cluster-wide metrics
func (c *K8sCollector) collectClusterMetrics(ctx context.Context) ([]Chart, error) {
	charts := make([]Chart, 0)

	// Namespace count
	namespaces, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err == nil {
		charts = append(charts, Chart{
			ID:        "k8s.cluster.namespaces",
			Type:      "k8s",
			Title:     "Namespace Count",
			Units:     "namespaces",
			Family:    "cluster",
			Context:   "k8s.cluster.namespaces",
			ChartType: "line",
			Priority:  10,
			Dimensions: []Dimension{
				{ID: "count", Name: "Namespaces", Value: float64(len(namespaces.Items))},
			},
		})
	}

	// Service count
	services, err := c.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err == nil {
		charts = append(charts, Chart{
			ID:        "k8s.cluster.services",
			Type:      "k8s",
			Title:     "Service Count",
			Units:     "services",
			Family:    "cluster",
			Context:   "k8s.cluster.services",
			ChartType: "line",
			Priority:  20,
			Dimensions: []Dimension{
				{ID: "count", Name: "Services", Value: float64(len(services.Items))},
			},
		})
	}

	// ConfigMap and Secret counts
	configmaps, err := c.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	if err == nil {
		secrets, _ := c.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
		secretCount := 0
		if secrets != nil {
			secretCount = len(secrets.Items)
		}

		charts = append(charts, Chart{
			ID:        "k8s.cluster.configs",
			Type:      "k8s",
			Title:     "ConfigMaps & Secrets",
			Units:     "objects",
			Family:    "cluster",
			Context:   "k8s.cluster.configs",
			ChartType: "line",
			Priority:  30,
			Dimensions: []Dimension{
				{ID: "configmaps", Name: "ConfigMaps", Value: float64(len(configmaps.Items))},
				{ID: "secrets", Name: "Secrets", Value: float64(secretCount)},
			},
		})
	}

	// PersistentVolumeClaim status
	pvcs, err := c.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err == nil {
		pvcStatus := map[string]float64{
			"Bound":   0,
			"Pending": 0,
			"Lost":    0,
		}

		for _, pvc := range pvcs.Items {
			phase := string(pvc.Status.Phase)
			pvcStatus[phase]++
		}

		pvcDims := make([]Dimension, 0)
		for status, count := range pvcStatus {
			pvcDims = append(pvcDims, Dimension{
				ID:    strings.ToLower(status),
				Name:  status,
				Value: count,
			})
		}

		charts = append(charts, Chart{
			ID:         "k8s.cluster.pvcs",
			Type:       "k8s",
			Title:      "PVC Status",
			Units:      "pvcs",
			Family:     "storage",
			Context:    "k8s.cluster.pvcs",
			ChartType:  "stacked",
			Priority:   40,
			Dimensions: pvcDims,
		})
	}

	return charts, nil
}

// push sends metrics to the alternative UI server
func (c *K8sCollector) push(charts []Chart) error {
	payload := MetricPush{
		NodeID:    c.clusterName,
		NodeName:  c.clusterName,
		OS:        "kubernetes",
		Timestamp: time.Now().UnixMilli(),
		Charts:    charts,
		Labels: map[string]string{
			"type": "kubernetes",
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.pushURL+"/api/v1/push", bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// sanitizeID converts a string to a valid ID
func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

func main() {
	pushURL := flag.String("url", "http://localhost:19998", "Alternative UI server URL")
	kubeconfig := flag.String("kubeconfig", "", "Path to kubeconfig file")
	apiKey := flag.String("api-key", "", "API key for authentication")
	clusterName := flag.String("cluster-name", "kubernetes", "Cluster name for identification")
	interval := flag.Duration("interval", 10*time.Second, "Collection interval")
	namespacesFlag := flag.String("namespaces", "", "Comma-separated list of namespaces to monitor (empty = all)")
	flag.Parse()

	var namespaces []string
	if *namespacesFlag != "" {
		namespaces = strings.Split(*namespacesFlag, ",")
	}

	log.Printf("Kubernetes Collector")
	log.Printf("  Push URL: %s", *pushURL)
	log.Printf("  Cluster: %s", *clusterName)
	log.Printf("  Interval: %s", *interval)
	if len(namespaces) > 0 {
		log.Printf("  Namespaces: %v", namespaces)
	} else {
		log.Printf("  Namespaces: all")
	}

	collector, err := NewK8sCollector(*kubeconfig, *pushURL, *apiKey, *clusterName, namespaces)
	if err != nil {
		log.Fatalf("Failed to create collector: %v", err)
	}

	ctx := context.Background()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	// Initial collection
	if err := collector.Collect(ctx); err != nil {
		log.Printf("Collection error: %v", err)
	} else {
		log.Printf("Initial collection successful")
	}

	for range ticker.C {
		if err := collector.Collect(ctx); err != nil {
			log.Printf("Collection error: %v", err)
		} else {
			log.Printf("[%s] Metrics pushed successfully", time.Now().Format("15:04:05"))
		}
	}
}
