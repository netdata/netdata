// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

// ----------------------------------------------------------------------------
// Windows timezone mapping
//
// c:\Windows\Globalization\Time Zone\TimezoneMapping.xml maps Windows zone IDs to IANA names, one row per line:
//   <MapTZ TZID="America/Los_Angeles" WinID="Pacific Standard Time" Region="US" Default="true" ... />
// The parser below is platform-independent, so it is unit-tested everywhere.

// Copy the value of attribute `name` from `line` into `dst`.
// Fails when the attribute is missing, its value is unterminated, or it does not fit - it never truncates.
static bool tz_mapping_attr(const char *line, const char *name, char *dst, size_t dst_size) {
    char key[32];
    snprintfz(key, sizeof(key), " %s=\"", name);

    const char *s = strstr(line, key);
    if (!s)
        return false;

    s += strlen(key);
    const char *e = strchr(s, '"');
    if (!e)
        return false;

    size_t len = (size_t)(e - s);
    if (len >= dst_size)
        return false;

    memcpy(dst, s, len);
    dst[len] = '\0';
    return true;
}

// Find the IANA name for `win_id`. Among the rows whose WinID equals it, the first row of the highest rank wins:
//   3 - Region is the user's region and Default="true"
//   2 - Region is the user's region
//   1 - Region is "001", the world default
// Returns 0 and writes `out` only when a TZID was found; otherwise returns -1 and leaves `out` untouched.
static int tz_windows_mapping_select(FILE *fp, const char *win_id, const char *geo, char *out, size_t out_size) {
    if (!fp || !win_id || !*win_id || !out || !out_size)
        return -1;

    if (!geo)
        geo = "";

    char line[CONFIG_FILE_LINE_MAX + 1];
    char best[FILENAME_MAX + 1] = "";
    int best_rank = 0;

    while (best_rank < 3 && fgets(line, sizeof(line), fp)) {
        size_t len = strlen(line);
        if (len && line[len - 1] != '\n') {
            // the buffer filled up or the file ended; the row is complete only if the next char ends the line
            int c = fgetc(fp);
            if (c != EOF && c != '\n') {
                // longer than our buffer: discard the rest of this line, never parse a partial row
                while ((c = fgetc(fp)) != EOF && c != '\n')
                    ;
                continue;
            }
        }

        char value[FILENAME_MAX + 1];
        if (!tz_mapping_attr(line, "WinID", value, sizeof(value)) || strcmp(value, win_id) != 0)
            continue;

        char region[16];
        if (!tz_mapping_attr(line, "Region", region, sizeof(region)))
            continue;

        int rank;
        if (*geo && strcmp(region, geo) == 0) {
            char is_default[8];
            rank = (tz_mapping_attr(line, "Default", is_default, sizeof(is_default)) &&
                    strcmp(is_default, "true") == 0) ? 3 : 2;
        }
        else if (strcmp(region, "001") == 0)
            rank = 1;
        else
            continue;

        if (rank <= best_rank)
            continue;

        if (!tz_mapping_attr(line, "TZID", value, MIN(sizeof(value), out_size)) || !*value)
            continue;

        strncpyz(best, value, sizeof(best) - 1);
        best_rank = rank;
    }

    // a read error may have hidden a better-ranked row; do not report a lower-ranked one as the answer
    if (ferror(fp) || !best_rank)
        return -1;

    strncpyz(out, best, out_size - 1);
    return 0;
}

#ifdef OS_WINDOWS
static void get_timezone_win_id(char *win_id, DWORD win_size)
{
    win_id[0] = '\0';
    HKEY hKey;
    LSTATUS ret = RegOpenKeyExA(HKEY_LOCAL_MACHINE,
                                "SYSTEM\\CurrentControlSet\\Control\\TimeZoneInformation",
                                0,
                                KEY_READ,
                                &hKey);
    if (ret != ERROR_SUCCESS)
        return;

    // the stored value is not guaranteed to be NUL-terminated, so keep a byte for it
    DWORD valueType = 0;
    DWORD size = win_size - 1;
    ret = RegQueryValueExA(hKey, "TimeZoneKeyName", NULL, &valueType, (LPBYTE)win_id, &size);
    RegCloseKey(hKey);

    if (ret != ERROR_SUCCESS || valueType != REG_SZ || size > win_size - 1) {
        win_id[0] = '\0';
        return;
    }

    win_id[size] = '\0';
}

static void get_win_geoiso(char *geo_name, int length) {
    GEOID id = GetUserGeoID(GEOCLASS_NATION);
    int res = GetGeoInfoA(id, GEO_ISO2, geo_name, length, 0);
    if (!res)
        geo_name[0] = '\0';
}

static int map_windows_tz_to_iana(char *out, size_t out_size, const char *win_id, const char *geo_name) {
    FILE *fp = fopen("c:\\Windows\\Globalization\\Time Zone\\TimezoneMapping.xml", "r");
    if (!fp)
        return -1;

    int ret = tz_windows_mapping_select(fp, win_id, geo_name, out, out_size);
    fclose(fp);
    return ret;
}
#endif

// Detect the current IANA timezone name from the system.
// Returns a pointer into the provided buffer, or NULL if detection fails.
const char *detect_system_timezone_name(char *buffer, size_t buffer_size) {
    if (!buffer || !buffer_size)
        return NULL;

    // callers may pass an uninitialised buffer; no path below may read it before writing it
    buffer[0] = '\0';
    const char *timezone = NULL;

#ifdef OS_WINDOWS
    char geo_name[128];
    char win_zone[256];
    get_timezone_win_id(win_zone, 256);
    get_win_geoiso(geo_name, 128);
    if (!map_windows_tz_to_iana(buffer, buffer_size, win_zone, geo_name))
        timezone = buffer;
#else
    // read the /etc/localtime symlink first — this is the authoritative source
    // on modern Linux (timedatectl always updates it, but /etc/timezone can lag)
    {
        ssize_t ret = readlink("/etc/localtime", buffer, buffer_size - 1);
        if (ret > 0) {
            buffer[ret] = '\0';

            const char *cmp = "/usr/share/zoneinfo/";
            size_t cmp_len = strlen(cmp);

            char *s = strstr(buffer, cmp);
            if (s && s[cmp_len])
                timezone = &s[cmp_len];
        }
    }

    // fall back to /etc/timezone (Debian/Ubuntu)
    if (!timezone && !read_txt_file("/etc/timezone", buffer, buffer_size)) {
        timezone = buffer;
    }
#endif

    if (timezone && *timezone) {
        // sanitize in-place: keep only alnum, '_', '/', '-', '+'
        char *d = buffer;
        const char *src = timezone;
        const char *src_end = buffer + buffer_size;
        const char *end = buffer + buffer_size - 1;
        while (src < src_end && *src && d < end) {
            if (isalnum((uint8_t)*src) || *src == '_' || *src == '/' || *src == '-' || *src == '+')
                *d++ = *src;
            src++;
        }
        *d = '\0';
        timezone = buffer;
    }

    return (timezone && *timezone) ? timezone : NULL;
}

static int timezone_windows_mapping_write_and_select(const char *rows, const char *win_id, const char *geo, char *out, size_t out_size) {
    FILE *fp = tmpfile();
    if (!fp)
        return -2;

    // a failed fixture write must fail the case, not let a no-match case pass without parsing anything
    if (fputs(rows, fp) == EOF || fflush(fp) != 0) {
        fclose(fp);
        return -2;
    }

    rewind(fp);
    int ret = tz_windows_mapping_select(fp, win_id, geo, out, out_size);
    fclose(fp);
    return ret;
}

int timezone_windows_mapping_unittest(void) {
#define PST "WinID=\"Pacific Standard Time\""
    static const char *pacific =
        "\xEF\xBB\xBF<?xml version=\"1.0\" encoding=\"utf-8\"?>\r\n"
        "<TimeZoneMapping>\r\n"
        "  <MapTZ TZID=\"Etc/GMT+8\" " PST " Region=\"001\" Default=\"true\" StdPath=\"PST/standard\" />\r\n"
        "  <MapTZ TZID=\"America/Vancouver\" " PST " Region=\"CA\" StdPath=\"PST/standard\" />\r\n"
        "  <MapTZ TZID=\"PST8PDT\" " PST " Region=\"US\" StdPath=\"PST/standard\" />\r\n"
        "  <MapTZ TZID=\"America/Los_Angeles\" " PST " Region=\"US\" Default=\"true\" StdPath=\"PST/standard\" />\r\n"
        "  <MapTZ TZID=\"Etc/UTC\" WinID=\"UTC\" Region=\"001\" Default=\"true\" StdPath=\"UTC/standard\" />\r\n"
        "</TimeZoneMapping>\r\n";
#undef PST

    static const struct {
        const char *name;
        const char *rows;
        const char *win_id;
        const char *geo;
        size_t out_size;
        int ret;
        const char *expected;
    } cases[] = {
        { "region with Default beats an earlier region row and 001", NULL, "Pacific Standard Time", "US", 64, 0, "America/Los_Angeles" },
        { "region without Default beats 001",                         NULL, "Pacific Standard Time", "CA", 64, 0, "America/Vancouver" },
        { "unknown region falls back to 001",                         NULL, "Pacific Standard Time", "GR", 64, 0, "Etc/GMT+8" },
        { "no region falls back to 001",                              NULL, "Pacific Standard Time", "",   64, 0, "Etc/GMT+8" },
        { "exact WinID match",                                        NULL, "UTC",                   "US", 64, 0, "Etc/UTC" },
        { "no match leaves out untouched",                            NULL, "Nowhere Standard Time", "US", 64, -1, "untouched" },
        { "empty win id",                                             NULL, "",                      "US", 64, -1, "untouched" },
        { "WinID must not match another attribute's value",
          "  <MapTZ TZID=\"UTC\" WinID=\"Coordinated\" Region=\"001\" StdPath=\"UTC\" />\n", "UTC", "", 64, -1, "untouched" },
        { "TZID that does not fit is rejected, not truncated",
          "  <MapTZ TZID=\"America/Argentina/ComodRivadavia\" WinID=\"X\" Region=\"001\" />\n", "X", "", 16, -1, "untouched" },
        { "unterminated TZID is rejected",
          "  <MapTZ WinID=\"X\" Region=\"001\" TZID=\"Europe/Athens\n", "X", "", 64, -1, "untouched" },
        { "row without Region is ignored",
          "  <MapTZ TZID=\"Europe/Athens\" WinID=\"X\" />\n", "X", "", 64, -1, "untouched" },
    };

    int failures = 0;
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        char out[64] = "untouched";
        int ret = timezone_windows_mapping_write_and_select(
            cases[i].rows ? cases[i].rows : pacific, cases[i].win_id, cases[i].geo, out, cases[i].out_size);

        if (ret != cases[i].ret || strcmp(out, cases[i].expected) != 0) {
            fprintf(stderr, "timezone_windows_mapping_unittest: '%s': expected %d '%s', got %d '%s'\n",
                    cases[i].name, cases[i].ret, cases[i].expected, ret, out);
            failures++;
        }
    }

    // a line longer than the read buffer is discarded whole; the row after it is still read
    {
        size_t junk = CONFIG_FILE_LINE_MAX + 100;
        const char *bad = " <MapTZ TZID=\"Bad/Zone\" WinID=\"X\" Region=\"001\" />\n";
        const char *good = "  <MapTZ TZID=\"Good/Zone\" WinID=\"X\" Region=\"001\" />\n";
        char *rows = mallocz(junk + strlen(bad) + strlen(good) + 1);
        memset(rows, 'x', junk);
        strcpy(rows + junk, bad);
        strcat(rows, good);

        char out[64] = "untouched";
        int ret = timezone_windows_mapping_write_and_select(rows, "X", "", out, sizeof(out));
        if (ret != 0 || strcmp(out, "Good/Zone") != 0) {
            fprintf(stderr, "timezone_windows_mapping_unittest: 'overlong line is discarded': expected 0 'Good/Zone', got %d '%s'\n",
                    ret, out);
            failures++;
        }
        freez(rows);
    }

    // a row of exactly sizeof(line) - 1 chars fills the read buffer but is complete, at EOF or before a newline
    for (int with_newline = 0; with_newline <= 1; with_newline++) {
        const char *head = "  <MapTZ TZID=\"Exact/Zone\" WinID=\"X\" Region=\"001\" StdPath=\"";
        const char *tail = "\" />";
        size_t row_len = CONFIG_FILE_LINE_MAX;
        char *rows = mallocz(row_len + 2);
        size_t pad = row_len - strlen(head) - strlen(tail);
        strcpy(rows, head);
        memset(rows + strlen(head), 'x', pad);
        strcpy(rows + strlen(head) + pad, tail);
        if (with_newline)
            strcat(rows, "\n");

        char out[64] = "untouched";
        int ret = timezone_windows_mapping_write_and_select(rows, "X", "", out, sizeof(out));
        if (ret != 0 || strcmp(out, "Exact/Zone") != 0) {
            fprintf(stderr, "timezone_windows_mapping_unittest: 'row filling the buffer exactly%s': expected 0 'Exact/Zone', got %d '%s'\n",
                    with_newline ? ", then a newline" : ", at EOF", ret, out);
            failures++;
        }
        freez(rows);
    }

    fprintf(stderr, "timezone_windows_mapping_unittest: %s (%d failures)\n", failures ? "FAILED" : "OK", failures);
    return failures;
}

// Set at startup: true when the user explicitly set "timezone" in netdata.conf.
static bool timezone_user_configured = false;

// True when the timezone name came from a proper source (config,
// /etc/localtime, /etc/timezone, TZ env var) rather than from the strftime("%Z")
// fallback which only produces bare abbreviations like "CEST" or "PST".
static bool timezone_is_tzdb_name = false;

static bool timezone_abbrev_normalize(char *dst, size_t dst_size, const char *src);

bool system_timezone_is_user_configured(void) {
    return timezone_user_configured;
}

bool system_timezone_is_tzdb_name(void) {
    return timezone_is_tzdb_name;
}

void get_system_timezone(void)
{
    char buffer[FILENAME_MAX + 1] = "";
    const char *timezone = NULL;

#ifndef OS_WINDOWS
    // avoid flood calls to stat(/etc/localtime)
    // http://stackoverflow.com/questions/4554271/how-to-avoid-excessive-stat-etc-localtime-calls-in-strftime-on-linux
    const char *tz = getenv("TZ");
    if (!tz || !*tz) {
        setenv("TZ", inicfg_get(&netdata_config, CONFIG_SECTION_ENV_VARS, "TZ", ":/etc/localtime"), 1);
        tz = getenv("TZ");
    }

    // use the TZ variable if it's an explicit IANA name (not a path starting with ':')
    if (tz && *tz && *tz != ':') {
        timezone = tz;
        timezone_is_tzdb_name = true;
        netdata_log_info("TIMEZONE: using TZ variable '%s'", timezone);
    }
#endif

    // Detect from system sources (/etc/localtime symlink, /etc/timezone, Windows API)
    if (!timezone) {
        timezone = detect_system_timezone_name(buffer, sizeof(buffer));
        if (timezone) {
            timezone_is_tzdb_name = true;
            netdata_log_info("TIMEZONE: detected '%s'", timezone);
        }
    }

    // Last resort: use strftime %Z (gives abbreviation, not IANA name).
    // timezone_is_tzdb_name stays false so refresh_system_timezone() won't
    // try to resolve it via tzalloc() or the tzfile parser.
    if (!timezone) {
        time_t t;
        struct tm *tmp, tmbuf;

        t = now_realtime_sec();
        tmp = localtime_r(&t, &tmbuf);

        if (tmp != NULL) {
            if (strftime(buffer, FILENAME_MAX, "%Z", tmp) != 0) {
                buffer[FILENAME_MAX] = '\0';
                timezone = buffer;
                netdata_log_info("TIMEZONE: using strftime(): '%s'", timezone);
            }
        }
    }

    if (!timezone || !*timezone)
        timezone = "unknown";

    // Check if the user explicitly set "timezone" in netdata.conf BEFORE
    // inicfg_get auto-populates it with the detected default.
    timezone_user_configured = inicfg_exists(&netdata_config, CONFIG_SECTION_GLOBAL, "timezone");

    char safe_timezone[FILENAME_MAX + 1];
    const char *default_timezone = timezone;

    if (timezone_is_tzdb_name) {
        if (!timezone_name_is_safe_tzdb_path(timezone)) {
            netdata_log_error("TIMEZONE: detected unsafe tzdb timezone '%s', ignoring", timezone);
            default_timezone = "unknown";
        }
    } else if (!timezone_abbrev_normalize(safe_timezone, sizeof(safe_timezone), timezone)) {
        default_timezone = "unknown";
    } else {
        default_timezone = safe_timezone;
    }

    // inicfg_get returns a config-system-owned pointer, stable for the process lifetime
    const char *configured_tz = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "timezone", default_timezone);

    // Treat "timezone =" (empty value) as not user-configured,
    // and fall back to the auto-detected timezone.
    if (timezone_user_configured && (!configured_tz || !*configured_tz)) {
        timezone_user_configured = false;
        configured_tz = default_timezone;
    }

    // If the user explicitly configured a timezone, treat it as a valid tzdb name
    // (the user is responsible for providing a valid value).
    if (timezone_user_configured)
        timezone_is_tzdb_name = true;

    // Compute abbreviation and UTC offset, then set all three atomically
    refresh_system_timezone(configured_tz, timezone_is_tzdb_name);
}

static inline uint32_t tzif_read_be32(const unsigned char *src) {
    return ((uint32_t)src[0] << 24) |
           ((uint32_t)src[1] << 16) |
           ((uint32_t)src[2] << 8)  |
           (uint32_t)src[3];
}

static inline int64_t tzif_read_be64(const unsigned char *src) {
    return (int64_t)(
        ((uint64_t)src[0] << 56) |
        ((uint64_t)src[1] << 48) |
        ((uint64_t)src[2] << 40) |
        ((uint64_t)src[3] << 32) |
        ((uint64_t)src[4] << 24) |
        ((uint64_t)src[5] << 16) |
        ((uint64_t)src[6] << 8)  |
        (uint64_t)src[7]
    );
}

bool timezone_name_is_safe_tzdb_path(const char *timezone) {
    if (!timezone || !*timezone || *timezone == '/')
        return false;

    for (const char *p = timezone; *p; p++) {
        if (!(isalnum((uint8_t)*p) || *p == '_' || *p == '/' || *p == '-' || *p == '+'))
            return false;
    }

    return true;
}

static bool timezone_abbrev_normalize(char *dst, size_t dst_size, const char *src) {
    if (!dst || dst_size < 2 || !src || !*src)
        return false;

    size_t len = 0;
    char *end = dst + dst_size - 1;

    while (*src && dst < end) {
        if (!(isalnum((uint8_t)*src) || *src == '_' || *src == '+' || *src == '-'))
            return false;

        *dst++ = *src++;
        len++;
    }

    *dst = '\0';
    return len != 0 && *src == '\0';
}

static bool timezone_info_from_tm(struct tm *tmp, char *abbrev, size_t abbrev_size, int32_t *offset) {
    if (!tmp)
        return false;

    int32_t new_offset = 0;
    char offset_str[16];

    if (strftime(abbrev, abbrev_size, "%Z", tmp) == 0)
        strncpyz(abbrev, "UTC", abbrev_size - 1);

    if (strftime(offset_str, sizeof(offset_str), "%z", tmp) != 0) {
        char sign = offset_str[0] == '-' || offset_str[0] == '+' ? offset_str[0] : '+';
        int hours = (isdigit((uint8_t)offset_str[1]) ? (offset_str[1] - '0') : 0) * 10 +
                    (isdigit((uint8_t)offset_str[2]) ? (offset_str[2] - '0') : 0);
        int minutes = (isdigit((uint8_t)offset_str[3]) ? (offset_str[3] - '0') : 0) * 10 +
                      (isdigit((uint8_t)offset_str[4]) ? (offset_str[4] - '0') : 0);

        new_offset = (hours * 3600) + (minutes * 60);
        if (sign == '-')
            new_offset = -new_offset;
    }

    *offset = new_offset;
    return true;
}

static bool current_process_timezone_info(time_t t, char *abbrev, size_t abbrev_size, int32_t *offset) {
    struct tm *tmp, tmbuf;
    tmp = localtime_r(&t, &tmbuf);
    return timezone_info_from_tm(tmp, abbrev, abbrev_size, offset);
}

#ifndef OS_WINDOWS
struct tzif_header {
    char magic[4];
    char version;
    char reserved[15];
    unsigned char ttisgmtcnt[4];
    unsigned char ttisstdcnt[4];
    unsigned char leapcnt[4];
    unsigned char timecnt[4];
    unsigned char typecnt[4];
    unsigned char charcnt[4];
};

struct tzif_type {
    int32_t gmtoff;
    uint8_t isdst;
    uint8_t abbrind;
};

static bool tzif_skip_bytes(FILE *fp, size_t bytes) {
    char buffer[256];
    while (bytes) {
        size_t chunk = bytes > sizeof(buffer) ? sizeof(buffer) : bytes;
        if (fread(buffer, 1, chunk, fp) != chunk)
            return false;
        bytes -= chunk;
    }
    return true;
}

// RFC 8536 sane upper bounds — real tzfiles are far smaller,
// but we allow headroom for unusual / future data.
#define TZIF_MAX_TIMECNT    2048
#define TZIF_MAX_TYPECNT     256
#define TZIF_MAX_CHARCNT    2048
#define TZIF_MAX_LEAPCNT      50
#define TZIF_MAX_ISSTDCNT    256
#define TZIF_MAX_ISGMTCNT   256

static bool tzif_validate_header_counts(uint32_t timecnt, uint32_t typecnt, uint32_t charcnt,
                                        uint32_t leapcnt, uint32_t ttisstdcnt, uint32_t ttisgmtcnt) {
    if (timecnt > TZIF_MAX_TIMECNT || typecnt > TZIF_MAX_TYPECNT ||
        charcnt > TZIF_MAX_CHARCNT || leapcnt > TZIF_MAX_LEAPCNT ||
        ttisstdcnt > TZIF_MAX_ISSTDCNT || ttisgmtcnt > TZIF_MAX_ISGMTCNT)
        return false;

    // ttisstdcnt and ttisgmtcnt must be 0 or equal to typecnt per RFC 8536
    if ((ttisstdcnt != 0 && ttisstdcnt != typecnt) ||
        (ttisgmtcnt != 0 && ttisgmtcnt != typecnt))
        return false;

    return true;
}

static bool tzif_skip_block(FILE *fp, const struct tzif_header *hdr, size_t time_size) {
    uint32_t ttisgmtcnt = tzif_read_be32(hdr->ttisgmtcnt);
    uint32_t ttisstdcnt = tzif_read_be32(hdr->ttisstdcnt);
    uint32_t leapcnt = tzif_read_be32(hdr->leapcnt);
    uint32_t timecnt = tzif_read_be32(hdr->timecnt);
    uint32_t typecnt = tzif_read_be32(hdr->typecnt);
    uint32_t charcnt = tzif_read_be32(hdr->charcnt);

    if (!tzif_validate_header_counts(timecnt, typecnt, charcnt, leapcnt, ttisstdcnt, ttisgmtcnt))
        return false;

    size_t bytes = (size_t)timecnt * time_size +
                   (size_t)timecnt +
                   (size_t)typecnt * 6 +
                   (size_t)charcnt +
                   (size_t)leapcnt * (time_size + 4) +
                   (size_t)ttisstdcnt +
                   (size_t)ttisgmtcnt;

    return tzif_skip_bytes(fp, bytes);
}

static int tzif_default_type_index(const struct tzif_type *types, uint32_t typecnt) {
    if (!types || !typecnt)
        return -1;

    for (uint32_t i = 0; i < typecnt; i++) {
        if (!types[i].isdst)
            return (int)i;
    }

    return 0;
}

static bool timezone_info_from_tzfile(const char *timezone, time_t t, char *abbrev, size_t abbrev_size, int32_t *offset) {
    if (!timezone_name_is_safe_tzdb_path(timezone))
        return false;

    const char *tzdir = getenv("TZDIR");
    if (!tzdir || !*tzdir)
        tzdir = "/usr/share/zoneinfo";

    char path[FILENAME_MAX + 1];
    snprintfz(path, sizeof(path), "%s/%s", tzdir, timezone);

    FILE *fp = fopen(path, "rb");
    if (!fp)
        return false;

    bool ok = false;
    struct tzif_header hdr;

    if (fread(&hdr, 1, sizeof(hdr), fp) != sizeof(hdr))
        goto cleanup;

    if (memcmp(hdr.magic, "TZif", 4) != 0)
        goto cleanup;

    if (hdr.version >= '2') {
        if (!tzif_skip_block(fp, &hdr, 4))
            goto cleanup;

        if (fread(&hdr, 1, sizeof(hdr), fp) != sizeof(hdr))
            goto cleanup;

        if (memcmp(hdr.magic, "TZif", 4) != 0)
            goto cleanup;
    }

    uint32_t timecnt = tzif_read_be32(hdr.timecnt);
    uint32_t typecnt = tzif_read_be32(hdr.typecnt);
    uint32_t charcnt = tzif_read_be32(hdr.charcnt);
    uint32_t leapcnt = tzif_read_be32(hdr.leapcnt);
    uint32_t ttisstdcnt = tzif_read_be32(hdr.ttisstdcnt);
    uint32_t ttisgmtcnt = tzif_read_be32(hdr.ttisgmtcnt);
    size_t time_size = (hdr.version >= '2') ? 8 : 4;

    if (!typecnt || !charcnt)
        goto cleanup;

    if (!tzif_validate_header_counts(timecnt, typecnt, charcnt, leapcnt, ttisstdcnt, ttisgmtcnt))
        goto cleanup;

    int64_t *transition_times = callocz(timecnt ? timecnt : 1, sizeof(*transition_times));
    uint8_t *transition_types = callocz(timecnt ? timecnt : 1, sizeof(*transition_types));
    struct tzif_type *types = callocz(typecnt, sizeof(*types));
    char *abbrs = callocz(charcnt + 1, sizeof(*abbrs));

    unsigned char timebuf[8];
    unsigned char typebuf[6];

    for (uint32_t i = 0; i < timecnt; i++) {
        if (fread(timebuf, 1, time_size, fp) != time_size)
            goto free_and_cleanup;
        transition_times[i] = (time_size == 8) ? tzif_read_be64(timebuf) : (int32_t)tzif_read_be32(timebuf);
    }

    if (timecnt && fread(transition_types, 1, timecnt, fp) != timecnt)
        goto free_and_cleanup;

    // validate all transition type indices before using them
    for (uint32_t i = 0; i < timecnt; i++) {
        if (transition_types[i] >= typecnt)
            goto free_and_cleanup;
    }

    for (uint32_t i = 0; i < typecnt; i++) {
        if (fread(typebuf, 1, sizeof(typebuf), fp) != sizeof(typebuf))
            goto free_and_cleanup;

        types[i].gmtoff = (int32_t)tzif_read_be32(typebuf);
        types[i].isdst = typebuf[4];
        types[i].abbrind = typebuf[5];
    }

    if (fread(abbrs, 1, charcnt, fp) != charcnt)
        goto free_and_cleanup;
    abbrs[charcnt] = '\0';

    int type_index = -1;
    if (timecnt == 0) {
        type_index = tzif_default_type_index(types, typecnt);
    } else {
        for (uint32_t i = 0; i < timecnt; i++) {
            if ((int64_t)t < transition_times[i])
                break;
            type_index = transition_types[i];
        }

        if (type_index < 0)
            type_index = tzif_default_type_index(types, typecnt);
    }

    if (type_index < 0 || (uint32_t)type_index >= typecnt)
        goto free_and_cleanup;

    if (types[type_index].abbrind >= charcnt)
        goto free_and_cleanup;

    const char *tz_abbrev = &abbrs[types[type_index].abbrind];
    if (!*tz_abbrev)
        tz_abbrev = "UTC";

    strncpyz(abbrev, tz_abbrev, abbrev_size - 1);
    *offset = types[type_index].gmtoff;
    ok = true;

free_and_cleanup:
    freez(abbrs);
    freez(types);
    freez(transition_types);
    freez(transition_times);

cleanup:
    fclose(fp);
    return ok;
}
#endif

void refresh_system_timezone(const char *timezone, bool is_tzdb_name) {
    time_t t;
    char abbrev[64];
    char safe_abbrev[64];
    const char *new_abbrev = "UTC";
    int32_t new_offset = 0;

    // Update global flag when we have confirmed tzdb knowledge.
    // When is_tzdb_name is false, preserve the existing global flag — a prior
    // successful detection may have already promoted it to true.
    if (is_tzdb_name)
        timezone_is_tzdb_name = true;

    t = now_realtime_sec();
    bool ok = false;

#if defined(HAVE_TZALLOC) && defined(HAVE_LOCALTIME_RZ) && defined(HAVE_TZFREE)
    if (is_tzdb_name) {
        timezone_t tz = tzalloc(timezone);
        if (tz) {
            struct tm *tmp, tmbuf;
            tmp = localtime_rz(tz, &t, &tmbuf);
            ok = timezone_info_from_tm(tmp, abbrev, sizeof(abbrev), &new_offset);
            tzfree(tz);
        }
    }
#elif !defined(OS_WINDOWS)
    if (is_tzdb_name)
        ok = timezone_info_from_tzfile(timezone, t, abbrev, sizeof(abbrev), &new_offset);
#endif

    if (!ok)
        ok = current_process_timezone_info(t, abbrev, sizeof(abbrev), &new_offset);

    if (ok && timezone_abbrev_normalize(safe_abbrev, sizeof(safe_abbrev), abbrev))
        new_abbrev = safe_abbrev;

    // Atomically update the system timezone triplet
    system_tz_set(timezone, new_abbrev, new_offset);

    // Update localhost if it exists
    if (localhost) {
        if (rrdhost_update_timezone(localhost, timezone, new_abbrev, new_offset)) {
            // Timezone changed — update the two labels directly, persist, and notify.
            bool labels_changed = false;
            if (localhost->rrdlabels) {
                labels_changed = rrdlabels_add_changed(localhost->rrdlabels, "_timezone", timezone, RRDLABEL_SRC_AUTO);
                labels_changed |= rrdlabels_add_changed(localhost->rrdlabels, "_abbrev_timezone", new_abbrev, RRDLABEL_SRC_AUTO);
            }

            if (labels_changed)
                rrdhost_labels_changed(localhost);
            else {
                // the host row and the node info carry the timezone themselves
                rrdhost_flag_set(localhost, RRDHOST_FLAG_METADATA_UPDATE);
                aclk_queue_node_info(localhost, false);
            }
        }
    }
}
