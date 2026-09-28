// SPDX-License-Identifier: GPL-3.0-or-later

//go:build scripts_native_dev

package main

// Native scripts remain opt-in until their development contract is ready to ship.
import _ "github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native"
