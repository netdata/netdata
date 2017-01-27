#ifndef NETDATA_WEB_BUFFER_H
#define NETDATA_WEB_BUFFER_H 1

/**
 * @file web_buffer.h
 * @brief Data structure and API of web_buffer.
 *
 * A web buffer is a buffer to store data for sending or recieving on web clients or servers.
 * A web buffer increases it's size automatic.
 * A web buffer is always null terminated.
 */

#define WEB_DATA_LENGTH_INCREASE_STEP 1024 ///< Minimum size a web buffer gets increased.

/** web buffer */
typedef struct web_buffer {
    size_t size;        	///< Total size of `buffer`, in bytes.
    size_t len;     		///< Used size of `buffer`, in bytes.
    char *buffer;   		///< The conetent
    uint8_t contenttype;	///< Content type of the buffer. (CT_*)
    uint8_t options;		///< WB_CONTENT_*
    time_t date;    		///< Timestamp the content has been created.
    time_t expires;			///< Timestamp the content expires.
} BUFFER;

// options
#define WB_CONTENT_CACHEABLE            1 ///< ktsaou: Your help needed
#define WB_CONTENT_NO_CACHEABLE         2 ///< ktsaou: Your help needed

// content-types
#define CT_APPLICATION_JSON             1  ///< MIME Type application/json
#define CT_TEXT_PLAIN                   2  ///< MIME Type text/plain
#define CT_TEXT_HTML                    3  ///< MIME Type text/html
#define CT_APPLICATION_X_JAVASCRIPT     4  ///< MIME Type application/javascript. \todo We should switch this to CT_APPLICATION_JAVASCRIPT
#define CT_TEXT_CSS                     5  ///< MIME Type text/css
#define CT_TEXT_XML                     6  ///< MIME Type text/xml
#define CT_APPLICATION_XML              7  ///< MIME Type application/xml
#define CT_TEXT_XSL                     8  ///< MIME Type text/xls. \todo We should switch to the standard type application/vnd.ms-excel here.
#define CT_APPLICATION_OCTET_STREAM     9  ///< MIME Type application/octet-stream
#define CT_APPLICATION_X_FONT_TRUETYPE  10 ///< MIME Type application/x-font-truetype \todo We should switch to the standard type font/ttf
#define CT_APPLICATION_X_FONT_OPENTYPE  11 ///< MIME Type application/x-font-opentype \todo We should switch to the standard type font/otf
#define CT_APPLICATION_FONT_WOFF        12 ///< MIME Type application/font-woff \todo We should switch to the standard type font/woff
#define CT_APPLICATION_FONT_WOFF2       13 ///< MIME Type application/font-woff2 \todo We should switch to the standard type font/woff2
#define CT_APPLICATION_VND_MS_FONTOBJ   14 ///< MIME Type application/vnd.ms-fontobject
#define CT_IMAGE_SVG_XML                15 ///< MIME Type image/svg+xml
#define CT_IMAGE_PNG                    16 ///< MIME Type image/png
#define CT_IMAGE_JPG                    17 ///< MIME Type image/jpg \todo We shoud switch to the standard type image/jpeg
#define CT_IMAGE_GIF                    18 ///< MIME Type image/gif
#define CT_IMAGE_XICON                  19 ///< MIME Type image/xicon. This type is not registred by IANA
#define CT_IMAGE_ICNS                   20 ///< MIME Type image/icns. This type is not registered by IANA
#define CT_IMAGE_BMP                    21 ///< MIME Type image/bmp
#define CT_PROMETHEUS                   22 ///< Content type prometheus. This is no valid MIME type.

/**
 * Set buffer cachable.
 *
 * @param wb The buffer.
 */
#define buffer_cacheable(wb)    do { (wb)->options |= WB_CONTENT_CACHEABLE;    if((wb)->options & WB_CONTENT_NO_CACHEABLE) (wb)->options &= ~WB_CONTENT_NO_CACHEABLE; } while(0)
/**
 * Set buffer not cacheable
 *
 * @param wb The Buffer.
 */
#define buffer_no_cacheable(wb) do { (wb)->options |= WB_CONTENT_NO_CACHEABLE; if((wb)->options & WB_CONTENT_CACHEABLE)    (wb)->options &= ~WB_CONTENT_CACHEABLE;  (wb)->expires = 0; } while(0)

/**
 * Get the length in bytes of the content of a web buffer.
 *
 * @param wb The Buffer.
 * @return length of `wb`
 */
#define buffer_strlen(wb) ((wb)->len)
/**
 * Get the content of web buffer `wb`
 *
 * The returned string is always null terminated.
 *
 * @param wb The buffer.
 * @return content of `wb`
 */
extern const char *buffer_tostring(BUFFER *wb);

/**
 * Ensure `buffer` has `needed_free_size` free bytes.
 *
 * Check if buffer has `needed_free_size`. If not increase the buffer.
 *
 * @param buffer to check
 * @param needed_free_size of `buffer`
 * 
 */
#define buffer_need_bytes(buffer, needed_free_size) do { if(unlikely((buffer)->size - (buffer)->len < (size_t)(needed_free_size))) buffer_increase((buffer), (size_t)(needed_free_size)); } while(0)

/**
 * Flush web_buffer `wb`
 *
 * Mark buffer as empty.
 *
 * @param wb Buffer to flush.
 */
#define buffer_flush(wb) wb->buffer[(wb)->len = 0] = '\0'
/**
 * Reset web_buffer `wb`
 *
 * Mark buffer as empty and reset metadata.
 *
 * @param wb Buffer to reset. 
 */
extern void buffer_reset(BUFFER *wb);

/**
 * Add `txt` to the end of web buffer `wb`.
 *
 * @param wb The buffer.
 * @param txt String to add.
 */
extern void buffer_strcat(BUFFER *wb, const char *txt);
/**
 * Add `value` to the end of web buffer `wb`.
 *
 * @param wb The buffer.
 * @param value to add
 */
extern void buffer_rrd_value(BUFFER *wb, calculated_number value);

/**
 * Add a date to web buffer `wb`.
 *
 * Format: `YYYY-MM-DD hh:mm:ss` (i.e. 2014-04-01 03:28:20)
 *
 * @param wb The buffer.
 * @param year of date
 * @param month of date
 * @param day of date
 * @param hours of date
 * @param minutes of date
 * @param seconds of date
 */
extern void buffer_date(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds);
/**
 * Add a date to web buffer `wb`.
 *
 * Format: `Date(YYYY,MM,DD,hh,mm,ss)` (i.e. Date(2014,04,01,03,28,20))
 *
 * @param wb The buffer.
 * @param year of date
 * @param month of date
 * @param day of date
 * @param hours of date
 * @param minutes of date
 * @param seconds of date
 */
extern void buffer_jsdate(BUFFER *wb, int year, int month, int day, int hours, int minutes, int seconds);

/**
 * Initialize a empty web buffer.
 *
 * @param size Initial size of the buffer.
 * @return the initialized buffer.
 */
extern BUFFER *buffer_create(size_t size);
/**
 * Free a web buffer.
 *
 * @param b The buffer.
 */
extern void buffer_free(BUFFER *b);
/**
 * Ensure `buffer` has `needed_free_size` free bytes.
 *
 * Use buffer_need_bytes() instead. It is quicker.
 *
 * \todo Do not extern this.
 *
 * Check if buffer has `needed_free_size`. If not increase the buffer.
 *
 * @param b buffer to check
 * @param free_size_required of `buffer`
 */
extern void buffer_increase(BUFFER *b, size_t free_size_required);

/**
 * Add at most `len` formated characters to web buffer `wb`.
 *
 * The formatter string and following parameters work like printf.
 * `wb` is always terminated with `\0`
 *
 * @see man 1 printf
 *
 * @param wb The buffer.
 * @param len Maximum number of characters to write.
 * @param fmt printf-like formatter string.
 */
extern void buffer_snprintf(BUFFER *wb, size_t len, const char *fmt, ...) PRINTFLIKE(3, 4);
/**
 * Add a formatted string to web buffer `wb`.
 *
 * The formatter string and following parameters work like printf.
 * `wb` is always terminated with `\0`
 *
 * @see man 1 printf
 *
 * @param wb The buffer.
 * @param fmt printf-like formatter string.
 * @param args to fill escape sequences with
 */
extern void buffer_vsprintf(BUFFER *wb, const char *fmt, va_list args);
/**
 * Add a formatted string to web buffer `wb`.
 *
 * The formatter string and following parameters work like printf.
 * `wb` is always terminated with `\0`
 *
 * @see man 1 printf
 *
 * @param wb The buffer.
 * @param fmt printf-like formatter string.
 */
extern void buffer_sprintf(BUFFER *wb, const char *fmt, ...) PRINTFLIKE(2,3);
/**
 * Add a string to web buffer `wb` with escaped html characters.
 * 
 * This replaces all of `&<>2/\` with their corresponding html string.
 *
 * @param wb The buffer.
 * @param txt String to add.
 */
extern void buffer_strcat_htmlescape(BUFFER *wb, const char *txt);

/**
 * Replace every character `from` in web buffer `wb` with `to`.
 *
 * @param wb The buffer.
 * @param from Character to replace.
 * @param to Character to insert.
 */
extern void buffer_char_replace(BUFFER *wb, char from, char to);

/**
 * ktsaou: Your help needed
 * 
 * @param str String.
 * @param uvalue value.
 * @return return.
 */
extern char *print_number_lu_r(char *str, unsigned long uvalue);
/**
 * ktsaou: Your help needed
 * 
 * @param str String.
 * @param uvalue value.
 * @return return.
 */
extern char *print_number_llu_r(char *str, unsigned long long uvalue);

/**
 * ktsaou: Your help needed
 * 
 * @param wb The buffer.
 * @param uvalue value.
 * @return return.
 */
extern void buffer_print_llu(BUFFER *wb, unsigned long long uvalue);

#endif /* NETDATA_WEB_BUFFER_H */
