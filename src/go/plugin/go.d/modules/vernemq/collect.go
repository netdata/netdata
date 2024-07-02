// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import (
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func isValidVerneMQMetrics(pms prometheus.Series) bool {
	return pms.FindByName(metricPUBLISHError).Len() > 0 && pms.FindByName(metricRouterSubscriptions).Len() > 0
}

func (v *VerneMQ) collect() (map[string]int64, error) {
	pms, err := v.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	if !isValidVerneMQMetrics(pms) {
		return nil, errors.New("returned metrics aren't VerneMQ metrics")
	}

	mx := v.collectVerneMQ(pms)

	return stm.ToMap(mx), nil
}

func (v *VerneMQ) collectVerneMQ(pms prometheus.Series) map[string]float64 {
	mx := make(map[string]float64)
	collectSockets(mx, pms)
	collectQueues(mx, pms)
	collectSubscriptions(mx, pms)
	v.collectErlangVM(mx, pms)
	collectBandwidth(mx, pms)
	collectRetain(mx, pms)
	collectCluster(mx, pms)
	collectUptime(mx, pms)

	v.collectAUTH(mx, pms)
	v.collectCONNECT(mx, pms)
	v.collectDISCONNECT(mx, pms)
	v.collectSUBSCRIBE(mx, pms)
	v.collectUNSUBSCRIBE(mx, pms)
	v.collectPUBLISH(mx, pms)
	v.collectPING(mx, pms)
	v.collectMQTTInvalidMsgSize(mx, pms)
	return mx
}

func (v *VerneMQ) collectCONNECT(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricCONNECTReceived,
		metricCONNACKSent,
	)
	v.collectMQTT(mx, pms)
}

func (v *VerneMQ) collectDISCONNECT(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricDISCONNECTReceived,
		metricDISCONNECTSent,
	)
	v.collectMQTT(mx, pms)
}

func (v *VerneMQ) collectPUBLISH(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricPUBACKReceived,
		metricPUBACKSent,
		metricPUBACKInvalid,

		metricPUBCOMPReceived,
		metricPUBCOMPSent,
		metricPUNCOMPInvalid,

		metricPUBSLISHReceived,
		metricPUBSLIHSent,
		metricPUBLISHError,
		metricPUBLISHAuthError,

		metricPUBRECReceived,
		metricPUBRECSent,
		metricPUBRECInvalid,

		metricPUBRELReceived,
		metricPUBRELSent,
	)
	v.collectMQTT(mx, pms)
}

func (v *VerneMQ) collectSUBSCRIBE(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricSUBSCRIBEReceived,
		metricSUBACKSent,
		metricSUBSCRIBEError,
		metricSUBSCRIBEAuthError,
	)
	v.collectMQTT(mx, pms)
}

func (v *VerneMQ) collectUNSUBSCRIBE(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricUNSUBSCRIBEReceived,
		metricUNSUBACKSent,
		metricUNSUBSCRIBEError,
	)
	v.collectMQTT(mx, pms)
}

func (v *VerneMQ) collectPING(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricPINGREQReceived,
		metricPINGRESPSent,
	)
	v.collectMQTT(mx, pms)
}

func (v *VerneMQ) collectAUTH(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricAUTHReceived,
		metricAUTHSent,
	)
	v.collectMQTT(mx, pms)
}

func (v *VerneMQ) collectMQTTInvalidMsgSize(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByName(metricMQTTInvalidMsgSizeError)
	v.collectMQTT(mx, pms)
}

func collectSockets(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricSocketClose,
		metricSocketCloseTimeout,
		metricSocketError,
		metricSocketOpen,
		metricClientKeepaliveExpired,
	)
	collectNonMQTT(mx, pms)
	mx["open_sockets"] = mx[metricSocketOpen] - mx[metricSocketClose]
}

func collectQueues(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricQueueInitializedFromStorage,
		metricQueueMessageDrop,
		metricQueueMessageExpired,
		metricQueueMessageIn,
		metricQueueMessageOut,
		metricQueueMessageUnhandled,
		metricQueueProcesses,
		metricQueueSetup,
		metricQueueTeardown,
	)
	collectNonMQTT(mx, pms)
}

func collectSubscriptions(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricRouterMatchesLocal,
		metricRouterMatchesRemote,
		metricRouterMemory,
		metricRouterSubscriptions,
	)
	collectNonMQTT(mx, pms)
}

func (v *VerneMQ) collectErlangVM(mx map[string]float64, pms prometheus.Series) {
	v.collectSchedulersUtilization(mx, pms)
	pms = pms.FindByNames(
		metricSystemContextSwitches,
		metricSystemGCCount,
		metricSystemIOIn,
		metricSystemIOOut,
		metricSystemProcessCount,
		metricSystemReductions,
		metricSystemRunQueue,
		metricSystemUtilization,
		metricSystemWordsReclaimedByGC,
		metricVMMemoryProcesses,
		metricVMMemorySystem,
	)
	collectNonMQTT(mx, pms)
}

func (v *VerneMQ) collectSchedulersUtilization(mx map[string]float64, pms prometheus.Series) {
	for _, pm := range pms {
		if isSchedulerUtilizationMetric(pm) {
			mx[pm.Name()] += pm.Value
			v.notifyNewScheduler(pm.Name())
		}
	}
}

func collectBandwidth(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricBytesReceived,
		metricBytesSent,
	)
	collectNonMQTT(mx, pms)
}

func collectRetain(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricRetainMemory,
		metricRetainMessages,
	)
	collectNonMQTT(mx, pms)
}

func collectCluster(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricClusterBytesDropped,
		metricClusterBytesReceived,
		metricClusterBytesSent,
		metricNetSplitDetected,
		metricNetSplitResolved,
	)
	collectNonMQTT(mx, pms)
	mx["netsplit_unresolved"] = mx[metricNetSplitDetected] - mx[metricNetSplitResolved]
}

func collectUptime(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByName(metricSystemWallClock)
	collectNonMQTT(mx, pms)
}

func collectNonMQTT(mx map[string]float64, pms prometheus.Series) {
	for _, pm := range pms {
		mx[pm.Name()] += pm.Value
	}
}

func (v *VerneMQ) collectMQTT(mx map[string]float64, pms prometheus.Series) {
	for _, pm := range pms {
		if !isMQTTMetric(pm) {
			continue
		}
		version := versionLabelValue(pm)
		if version == "" {
			continue
		}

		mx[pm.Name()] += pm.Value
		mx[join(pm.Name(), "v", version)] += pm.Value

		if reason := reasonCodeLabelValue(pm); reason != "" {
			mx[join(pm.Name(), reason)] += pm.Value
			mx[join(pm.Name(), "v", version, reason)] += pm.Value

			v.notifyNewReason(pm.Name(), reason)
		}
	}
}

func isMQTTMetric(pm prometheus.SeriesSample) bool {
	return strings.HasPrefix(pm.Name(), "mqtt_")
}

func isSchedulerUtilizationMetric(pm prometheus.SeriesSample) bool {
	return strings.HasPrefix(pm.Name(), "system_utilization_scheduler_")
}

func reasonCodeLabelValue(pm prometheus.SeriesSample) string {
	if v := pm.Labels.Get("reason_code"); v != "" {
		return v
	}
	// "mqtt_connack_sent" v4 has return_code
	return pm.Labels.Get("return_code")
}

func versionLabelValue(pm prometheus.SeriesSample) string {
	return pm.Labels.Get("mqtt_version")
}

func join(a, b string, rest ...string) string {
	v := a + "_" + b
	switch len(rest) {
	case 0:
		return v
	default:
		return join(v, rest[0], rest[1:]...)
	}
}
