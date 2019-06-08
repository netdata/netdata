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

#define WEB_FIELDS_MAX 200
struct web_fields{
    char *body;
    size_t length;
};
// http_request_validate()
// returns:
// = 0 : all good, process the request
// > 0 : request is not supported
// < 0 : request is incomplete - wait for more data

typedef enum {
    HTTP_VALIDATION_OK,
    HTTP_VALIDATION_NOT_SUPPORTED,
    HTTP_VALIDATION_INCOMPLETE,
    HTTP_VALIDATION_REDIRECT
} HTTP_VALIDATION;

extern HTTP_VALIDATION url_is_request_complete(char *begin,char *end,size_t length);
extern char *url_find_protocol(char *s);
extern int url_parse_query_string(struct web_fields *names,struct web_fields *values,char *moveme,char *divisor);

#endif /* NETDATA_URL_H */
