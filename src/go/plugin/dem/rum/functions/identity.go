// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import "strconv"

// Prefix the site's byte length so arbitrary site-local values cannot collide.
func rowID(site, value string) string { return strconv.Itoa(len(site)) + ":" + site + value }
