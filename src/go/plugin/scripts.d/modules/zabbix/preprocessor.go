// SPDX-License-Identifier: GPL-3.0-or-later

package zabbix

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	zpre "github.com/netdata/netdata/go/plugins/pkg/zabbixpreproc"
)

var (
	preprocOnce sync.Once
	sharedProc  *zpre.Preprocessor
)

func acquirePreprocessor() *zpre.Preprocessor {
	preprocOnce.Do(func() {
		ns := buildPreprocessorNamespace()
		sharedProc = zpre.NewPreprocessor(ns)
	})
	return sharedProc
}

func buildPreprocessorNamespace() string {
	if host, err := os.Hostname(); err == nil {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			return fmt.Sprintf("zabbix-%s", trimmed)
		}
	}
	return fmt.Sprintf("zabbix-%d", time.Now().UnixNano())
}
