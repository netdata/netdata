/* vim: set ts=4 noet sw=4 : */
#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

#include "common.h"
#include "log.h"
#include "url.h"

// ----------------------------------------------------------------------------
// URL encode / decode
// code from: http://www.geekhideout.com/urlcode.shtml

/* Converts a hex character to its integer value */
char from_hex(char ch) {
	return (char)(isdigit(ch) ? ch - '0' : tolower(ch) - 'a' + 10);
}

/* Converts an integer value to its hex character*/
char to_hex(char code) {
	static char hex[] = "0123456789abcdef";
	return hex[code & 0x0f];
}

/* Returns a url-encoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_encode(char *str) {
	char *pstr, *buf, *pbuf;

	pstr = str;
	pbuf = buf = malloc(strlen(str) * 3 + 1);
	
	if(!buf)
		fatal("Cannot allocate memory.");

	for (; *pstr; pstr++, pbuf++)
		if (isalnum(*pstr) || *pstr == '-' || *pstr == '_' || *pstr == '.' || *pstr == '~')
			*pbuf = *pstr;
		else if (*pstr == ' ')
			*pbuf = '+';
		else {
			/* NOTE: Sometimes using , operator can lead to ambiguous code. */
			*pbuf++ = '%';
			*pbuf++ = to_hex(*pstr >> 4);
			*pbuf = to_hex(*pstr);	// FIXME: to_hex already isolates the 4 lsb bits.
		}
	*pbuf = '\0';

	return buf;
}

/* Returns a url-decoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_decode(char *str) {
	char *pstr, *buf, *pbuf;

	pstr = str;
	pbuf = buf = strdup(str);

	if(!buf)
		fatal("Cannot allocate memory.");

	for (; *pstr; pstr++)
		/* NOTE: Let the compiler find the best comparison strategy. */
		switch (*pstr) {
		case '%':
			if (pstr[1] && pstr[2]) {
				*pbuf++ = (from_hex(pstr[1]) << 4) | from_hex(pstr[2]);
				pstr += 2;
			}
			break;
		case '+':
			*pbuf++ = ' ';
			break;
		default:
			*pbuf++ = *pstr;
		}

	*pbuf = '\0';

	return buf;
}
