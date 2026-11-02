// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HTTP_HEADER_H
#define NETDATA_HTTP_HEADER_H

#include "web_api.h"

struct web_client;

// Parses the header line at s and calls its handler with the value, without the whitespace around it.
// Returns the line's terminating '\r' (or the end of the string), or NULL when the line is malformed:
// it has no colon, or it is an obsolete folded continuation line.
char *http_header_parse_line(struct web_client *w, char *s);

#endif //NETDATA_HTTP_HEADER_H
