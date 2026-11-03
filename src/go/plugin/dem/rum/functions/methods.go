// SPDX-License-Identifier: GPL-3.0-or-later
package functions

type method struct {
	id, title, help, sort string
	every                 int
	history               bool
	params                []string
	columns               map[string]any
}

var methods = []method{sitesMethod, pagesMethod, liveMethod, sessionsMethod, errorsMethod, sessionEventsMethod}
