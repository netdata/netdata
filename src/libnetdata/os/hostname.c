// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#ifdef HAVE_LIBICONV
#include <iconv.h>

static const char *get_current_locale(void) {
    const char *locale = getenv("LC_ALL");  // LC_ALL overrides all other locale settings

    if (!locale || !*locale) {
        locale = getenv("LC_CTYPE");  // Check LC_CTYPE for character encoding

        if (!locale || !*locale)
            locale = getenv("LANG");  // Fallback to LANG
    }

    return locale;
}

static const char *get_encoding_from_locale(const char *locale) {
    if(!locale || !*locale)
        return NULL;

    const char *dot = strchr(locale, '.');
    if (dot)
        return dot + 1;

    return locale;
}

// Function to convert from a source encoding to UTF-8
static bool iconv_convert_to_utf8(const char *input, const char *src_encoding, char *output, size_t output_size) {
    iconv_t cd = iconv_open("UTF-8", src_encoding);
    if (cd == (iconv_t)-1) {
        int i = errno;
        return false;
    }

    char *input_ptr = (char *)input;    // iconv() may modify this pointer
    char *output_ptr = output;          // iconv() modifies this pointer
    size_t input_len = strlen(input);
    size_t output_len = output_size;

    // Perform the conversion
    if (iconv(cd, &input_ptr, &input_len, &output_ptr, &output_len) == (size_t)-1) {
        iconv_close(cd);
        return false;
    }

    // Null-terminate the output string
    *output_ptr = '\0';

    iconv_close(cd);
    return true;
}
#endif

bool os_hostname(char *dst, size_t dst_size, const char *filesystem_root) {
    *dst = '\0';

    char buf[HOST_NAME_MAX * 4 + 1];
    *buf = '\0';

    if (filesystem_root && *filesystem_root) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/etc/hostname", netdata_configured_host_prefix);

        if (read_txt_file(filename, buf, sizeof(buf)))
            *buf = '\0';
    }

    if(!*buf && gethostname(buf, sizeof(buf)) != 0)
        snprintf(buf, sizeof(buf), "host%ld", gethostid());

    char *original_hostname = trim(buf);

#ifdef HAVE_LIBICONV
    const char *locale = get_current_locale();
    if (locale && *locale) {
        char utf8_output[HOST_NAME_MAX * 4 + 1] = "";
        if(iconv_convert_to_utf8(original_hostname, get_encoding_from_locale(locale), utf8_output, sizeof(utf8_output))) {
            rrdlabels_sanitize_value(dst, trim(utf8_output), dst_size);
            return *dst != '\0';
        }
    }
#endif

    rrdlabels_sanitize_value(dst, original_hostname, dst_size);
    return *dst != '\0';
}
