// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !cgo || !ibm_mq

package mq

func (c *Collector) collectSubscriptions() error {
	c.Debugf("Subscription collection requires CGO support")
	return nil
}
