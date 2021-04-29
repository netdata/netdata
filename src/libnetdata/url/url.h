// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_URL_H
#define NETDATA_URL_H 1

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// URL encode / decode
// code from: http://www.geekhideout.com/urlcode.shtml

/* Converts a hex character to its integer value */
extern char from_hex(char ch);

/* Converts an integer value to its hex character*/
extern char to_hex(char code);

/* Returns a url-encoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
extern char *url_encode(char *str);

/* Returns a url-decoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
extern char *url_decode(char *str);

extern char *url_decode_r(char *to, char *url, size_t size);

#define WEB_FIELDS_MAX 400
extern int url_map_query_string(char **out, char *url);
extern int url_parse_query_string(char *output, size_t max, char **map, int total);

extern int url_is_request_complete(char *begin,char *end,size_t length);
extern char *url_find_protocol(char *s);

#endif /* NETDATA_URL_H */
