// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

const char *netdata_configured_hostname            = NULL;
const char *netdata_configured_user_config_dir     = CONFIG_DIR;
const char *netdata_configured_stock_config_dir    = LIBCONFIG_DIR;
const char *netdata_configured_stock_data_dir      = STOCK_DATA_DIR;
const char *netdata_configured_log_dir             = LOG_DIR;
const char *netdata_configured_primary_plugins_dir = PLUGINS_DIR;
const char *netdata_configured_web_dir             = WEB_DIR;
const char *netdata_configured_cache_dir           = CACHE_DIR;
const char *netdata_configured_varlib_dir          = VARLIB_DIR;
const char *netdata_configured_cloud_dir           = VARLIB_DIR "/cloud.d";
const char *netdata_configured_home_dir            = VARLIB_DIR;
const char *netdata_configured_host_prefix         = NULL;

bool netdata_ready = false;

// ============================================================================
// system timezone - thread-safe access

static struct {
    SPINLOCK spinlock;
    char *timezone;
    char *abbrev_timezone;
    int32_t utc_offset;
} system_tz = {
    .spinlock = SPINLOCK_INITIALIZER,
    .timezone = NULL,
    .abbrev_timezone = NULL,
    .utc_offset = 0,
};

void system_tz_set(const char *timezone, const char *abbrev_timezone, int32_t utc_offset) {
    // Own copies of both strings
    char *new_tz = strdupz(timezone ? timezone : "unknown");
    char *new_abbrev = strdupz(abbrev_timezone ? abbrev_timezone : "UTC");

    spinlock_lock(&system_tz.spinlock);
    // All readers use system_tz_get() which holds this same spinlock and copies,
    // so no reader can be using these pointers after we release the lock.
    char *old_tz = system_tz.timezone;
    char *old_abbrev = system_tz.abbrev_timezone;
    system_tz.timezone = new_tz;
    system_tz.abbrev_timezone = new_abbrev;
    system_tz.utc_offset = utc_offset;
    spinlock_unlock(&system_tz.spinlock);

    freez(old_tz);
    freez(old_abbrev);
}

SYSTEM_TZ system_tz_get(void) {
    SYSTEM_TZ tz;
    spinlock_lock(&system_tz.spinlock);
    tz.timezone = strdupz(system_tz.timezone ? system_tz.timezone : "unknown");
    tz.abbrev_timezone = strdupz(system_tz.abbrev_timezone ? system_tz.abbrev_timezone : "UTC");
    tz.utc_offset = system_tz.utc_offset;
    spinlock_unlock(&system_tz.spinlock);
    return tz;
}

void system_tz_free(SYSTEM_TZ *tz) {
    freez(tz->timezone);
    freez(tz->abbrev_timezone);
    tz->timezone = NULL;
    tz->abbrev_timezone = NULL;
    tz->utc_offset = 0;
}
