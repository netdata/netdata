#ifndef NETDATA_URL_H
#define NETDATA_URL_H 1

/**
 * @file url.h
 * @brief URL encode / decode
 * @see code from: http://www.geekhideout.com/urlcode.shtml
 */

/**
 * Converts a hex character to its integer value.
 *
 * @param ch Hex character.
 * @return Integer Value
 */
extern char from_hex(char ch);

/**
 * Converts an integer value to its hex character.
 *
 * @param code Integer code.
 * @return Hex character
 */
extern char to_hex(char code);

/** 
 *Returns a url-encoded version of `str`
 *
 * IMPORTANT: be sure to `free()` the returned string after use 
 *
 * @param str to encode
 * @return url-encoded string
 */
extern char *url_encode(char *str);

/**
 * Returns a url-decoded version of `str`.
 *
 * IMPORTANT: be sure to free() the returned string after use
 *
 * @param str to decode
 * @return url-decoded string
 */
extern char *url_decode(char *str);

/**
 * Returns a url-decoded version of `str`.
 *
 * Decodes at most `size` bytes.
 * IMPORTANT: be sure to free() the returned string after use.
 *
 * @param to destiantion
 * @param url source
 * @param size bytes to copy at most
 * @return url-decoded string
 */
extern char *url_decode_r(char *to, char *url, size_t size);

#endif /* NETDATA_URL_H */
