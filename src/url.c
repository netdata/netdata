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
	return hex[code & 15];
}

/* Returns a url-encoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_encode(char *str) {
	char *buf, *pbuf;

	pbuf = buf = malloc(strlen(str) * 3 + 1);

	if(!buf)
		fatal("Cannot allocate memory.");

	while (*str) {
		if (isalnum(*str) || *str == '-' || *str == '_' || *str == '.' || *str == '~')
			*pbuf++ = *str;

		else if (*str == ' ')
			*pbuf++ = '+';

		else
			*pbuf++ = '%', *pbuf++ = to_hex(*str >> 4), *pbuf++ = to_hex(*str & 15);

		str++;
	}
	*pbuf = '\0';

	// FIX: I think this is prudent. URLs can be as long as 2 KiB or more.
	//      We allocated 3 times more space to accomodate %NN encoding of
	//      non ASCII chars. If URL has none of these kind of chars we will
	//      end up with a big unused buffer.
	//
	//      Try to shrink the buffer...
	if (!!(pbuf = (char *)realloc(buf, strlen(buf)+1)))
		buf = pbuf;

	return buf;
}

/* Returns a url-decoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_decode(char *str) {
	size_t size = strlen(str) + 1;

	char *buf = malloc(size);
	if(!buf)
		fatal("Cannot allocate %zu bytes of memory.", size);

	return url_decode_r(buf, str, size);
}

char *url_decode_r(char *to, char *url, size_t size) {
	char *s = url,           // source
		 *d = to,            // destination
		 *e = &to[size - 1]; // destination end

	while(*s && d < e) {
		if(unlikely(*s == '%')) {
			if(likely(s[1] && s[2])) {
				*d++ = from_hex(s[1]) << 4 | from_hex(s[2]);
				s += 2;
			}
		}
		else if(unlikely(*s == '+'))
			*d++ = ' ';

		else
			*d++ = *s;

		s++;
	}

	*d = '\0';

	return to;
}
