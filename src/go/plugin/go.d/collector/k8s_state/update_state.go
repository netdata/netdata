// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

func (c *Collector) runUpdateState(in <-chan resource) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case res := <-in:
			c.state.Lock()
			switch res.kind() {
			case kubeResourceNode:
				c.updateNodeState(res)
			case kubeResourcePod:
				c.updatePodState(res)
			case kubeResourceDeployment:
				c.updateDeploymentState(res)
			case kubeResourceCronJob:
				c.updateCronJobState(res)
			case kubeResourceJob:
				c.updateJobState(res)
			}
			c.state.Unlock()
		}
	}
}

func copyLabels(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

func ptr[T any](v T) *T {
	return &v
}
