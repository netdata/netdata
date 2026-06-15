// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

type TrapWriter interface {
	Write(entry *TrapEntry) error
	Flush() error
	Close() error
}
