// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HTTP_HEADER_H
#define NETDATA_HTTP_HEADER_H

#include "web_api.h"

struct web_client;

// Parses the header line at s and calls its handler with the value, without the whitespace around it.
// Returns the line's terminating '\r' (or the end of the string, while the request is still arriving), or NULL
// when the line is malformed (see http_header_line_check() for the rules).
char *http_header_parse_line(struct web_client *w, char *s);

// Checks every header line of a request whose header section is complete, without calling any handler.
// Returns false when a line is malformed, true when all are valid or the section is not complete yet.
bool http_header_section_is_valid(const char *request);

#endif //NETDATA_HTTP_HEADER_H
