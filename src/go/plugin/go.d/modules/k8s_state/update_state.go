// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

func (ks *KubeState) runUpdateState(in <-chan resource) {
	for {
		select {
		case <-ks.ctx.Done():
			return
		case r := <-in:
			ks.state.Lock()
			switch r.kind() {
			case kubeResourceNode:
				ks.updateNodeState(r)
			case kubeResourcePod:
				ks.updatePodState(r)
			}
			ks.state.Unlock()
		}
	}
}

func copyLabels(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}
