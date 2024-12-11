// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_URL_H
#define NETDATA_URL_H 1

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// URL encode / decode
// code from: http://www.geekhideout.com/urlcode.shtml

/* Converts a hex character to its integer value */
char from_hex(char ch);

/* Converts an integer value to its hex character*/
char to_hex(char code);

/* Returns a url-encoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_encode(const char *str);

/* Returns a url-decoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_decode(char *str);

char *url_decode_r(char *to, const char *url, size_t size);

bool url_is_request_complete_and_extract_payload(const char *begin, const char *end, size_t length, BUFFER **post_payload);
char *url_find_protocol(char *s);

#endif /* NETDATA_URL_H */
