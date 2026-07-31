// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

func (d functionDeps) AnyBackendAvailable() bool {
	return d.runtime.AnyBackendAvailable()
}

func (d functionDeps) LogRoots() map[string]string {
	return d.runtime.LogRoots()
}
