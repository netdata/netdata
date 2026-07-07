// SPDX-License-Identifier: GPL-3.0-or-later

#include "boot_id.h"
#include "libnetdata/libnetdata.h"

static ND_UUID cached_boot_id = { 0 };
static bool cached_boot_id_available = false;
static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

#if defined(OS_LINUX)

static ND_UUID get_boot_id(void) {
    ND_UUID boot_id = { 0 };
    char buf[UUID_STR_LEN];

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/proc/sys/kernel/random/boot_id",
              netdata_configured_host_prefix ? netdata_configured_host_prefix : "");

    // Try reading the official boot_id first
    if (read_txt_file(filename, buf, sizeof(buf)) == 0) {
        if (uuid_parse(trim(buf), boot_id.uuid) == 0)
            return boot_id;
    }

    // Fallback to boottime-based ID
    time_t boottime = os_boottime();
    if(boottime > 0) {
        boot_id.parts.low64 = (uint64_t)boottime;
        // parts.hig64 remains 0 to indicate this is a synthetic boot_id
    }

    return boot_id;
}

#else // !OS_LINUX

static ND_UUID get_boot_id(void) {
    ND_UUID boot_id = { 0 };

    time_t boottime = os_boottime();
    if(boottime > 0) {
        boot_id.parts.low64 = (uint64_t)boottime;
        // parts.hig64 remains 0 to indicate this is a synthetic boot_id
    }

    return boot_id;
}

#endif // OS_LINUX

ND_UUID os_boot_id(void) {
    if(__atomic_load_n(&cached_boot_id_available, __ATOMIC_ACQUIRE))
        return cached_boot_id;

    spinlock_lock(&spinlock);

    ND_UUID boot_id = cached_boot_id;
    if(UUIDiszero(boot_id)) {
        boot_id = get_boot_id();
        cached_boot_id = boot_id;

        if(!UUIDiszero(boot_id))
            __atomic_store_n(&cached_boot_id_available, true, __ATOMIC_RELEASE);
    }

    spinlock_unlock(&spinlock);
    return boot_id;
}

bool os_boot_ids_match(ND_UUID a, ND_UUID b) {
    if(UUIDeq(a, b))
        return true;

    if(a.parts.hig64 == 0 && b.parts.hig64 == 0) {
        uint64_t diff = a.parts.low64 > b.parts.low64 ? a.parts.low64 - b.parts.low64 : b.parts.low64 - a.parts.low64;
        if(diff <= 3)
            return true;
    }

    return false;
}
