// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

func parseTemplate(raw string) (program.Template, error) {
	tpl := program.Template{
		Raw: strings.TrimSpace(raw),
	}
	if tpl.Raw == "" {
		return tpl, fmt.Errorf("template cannot be empty")
	}
	return tpl, nil
}
