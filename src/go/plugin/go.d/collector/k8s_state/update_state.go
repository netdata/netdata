// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

func (c *Collector) runUpdateState(in <-chan resource) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case r := <-in:
			c.state.Lock()
			switch r.kind() {
			case kubeResourceNode:
				c.updateNodeState(r)
			case kubeResourcePod:
				c.updatePodState(r)
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
