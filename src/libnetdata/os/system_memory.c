// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// Windows
#if defined(OS_WINDOWS)
#include <windows.h>

OS_SYSTEM_MEMORY os_system_memory(bool query_total_ram __maybe_unused) {
    OS_SYSTEM_MEMORY sm = {0, 0};

    MEMORYSTATUSEX statex;
    statex.dwLength = sizeof(statex);
    if (GlobalMemoryStatusEx(&statex)) {
        sm.ram_total_bytes = statex.ullTotalPhys;
        sm.ram_available_bytes = statex.ullAvailPhys;
    }

    return sm;
}
#endif

// macOS
#if defined(OS_MACOS)
#include <mach/mach.h>
#include <sys/sysctl.h>

OS_SYSTEM_MEMORY os_system_memory(bool query_total_ram) {
    static uint64_t total_ram = 0;
    static uint64_t page_size = 0;

    if (page_size == 0) {
        size_t len = sizeof(page_size);
        if (sysctlbyname("hw.pagesize", &page_size, &len, NULL, 0) != 0)
            return (OS_SYSTEM_MEMORY){ 0, 0 };
    }

    if (query_total_ram || total_ram == 0) {
        size_t len = sizeof(total_ram);
        if (sysctlbyname("hw.memsize", &total_ram, &len, NULL, 0) != 0)
            return (OS_SYSTEM_MEMORY){ 0, 0 };
    }

    uint64_t ram_available = 0;
    if (page_size > 0) {
        vm_statistics64_data_t vm_info;
        mach_msg_type_number_t count = HOST_VM_INFO64_COUNT;
        mach_port_t mach_port = mach_host_self();

        if (host_statistics64(mach_port, HOST_VM_INFO64, (host_info_t)&vm_info, &count) != KERN_SUCCESS) {
            mach_port_deallocate(mach_task_self(), mach_port);
            return (OS_SYSTEM_MEMORY){0, 0};
        }

        ram_available = (vm_info.free_count + vm_info.inactive_count + vm_info.purgeable_count) * page_size;
        mach_port_deallocate(mach_task_self(), mach_port);
    }

    return (OS_SYSTEM_MEMORY){
        .ram_total_bytes = total_ram,
        .ram_available_bytes = ram_available,
    };
}
#endif

// Linux
#if defined(OS_LINUX)

static OS_SYSTEM_MEMORY os_system_memory_cgroup_v1(bool query_total_ram __maybe_unused) {
    static OS_SYSTEM_MEMORY sm = {0, 0};
    char buf[64];

    if(query_total_ram || sm.ram_total_bytes == 0) {
        if (read_txt_file("/sys/fs/cgroup/memory/memory.limit_in_bytes", buf, sizeof(buf)) != 0)
            goto failed;

        sm.ram_total_bytes = strtoull(buf, NULL, 10);
        if(!sm.ram_total_bytes)
            goto failed;
    }

    buf[0] = '\0';
    if (read_txt_file("/sys/fs/cgroup/memory/memory.usage_in_bytes", buf, sizeof(buf)) != 0)
        goto failed;

    uint64_t used = strtoull(buf, NULL, 10);
    if(!used || used > sm.ram_total_bytes)
        goto failed;

    sm.ram_available_bytes = sm.ram_total_bytes - used;
    return sm;

failed:
    sm.ram_total_bytes = 0;
    sm.ram_available_bytes = 0;
    return sm;
}

static OS_SYSTEM_MEMORY os_system_memory_cgroup_v2(bool query_total_ram __maybe_unused) {
    static OS_SYSTEM_MEMORY sm = {0, 0};
    char buf[64];

    if(query_total_ram || sm.ram_total_bytes == 0) {
        if (read_txt_file("/sys/fs/cgroup/memory.max", buf, sizeof(buf)) != 0)
            goto failed;

        if(strcmp(buf, "max") == 0)
            sm.ram_total_bytes = UINT64_MAX;
        else
            sm.ram_total_bytes = strtoull(buf, NULL, 10);

        if(!sm.ram_total_bytes)
            goto failed;
    }

    buf[0] = '\0';
    if (read_txt_file("/sys/fs/cgroup/memory.current", buf, sizeof(buf)) != 0)
        goto failed;

    uint64_t used = strtoull(buf, NULL, 10);
    if(!used || used > sm.ram_total_bytes)
        goto failed;

    sm.ram_available_bytes = sm.ram_total_bytes - used;
    return sm;

failed:
    sm.ram_total_bytes = 0;
    sm.ram_available_bytes = 0;
    return sm;
}

OS_SYSTEM_MEMORY os_system_memory_meminfo(bool query_total_ram __maybe_unused) {
    static OS_SYSTEM_MEMORY sm = {0, 0};
    static procfile *ff = NULL;

    if(unlikely(!ff)) {
        ff = procfile_open("/proc/meminfo", ": \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff))
            goto failed;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        goto failed;

    bool matched_total = false, matched_available = false;
    size_t lines = procfile_lines(ff);
    for(size_t line = 0; line < lines ;line++) {
        if(!matched_total && strcmp(procfile_lineword(ff, line, 0), "MemTotal") == 0) {
            sm.ram_total_bytes = str2ull(procfile_lineword(ff, line, 1), NULL) * 1024;
            matched_total = true;
        }

        if(!matched_available && strcmp(procfile_lineword(ff, line, 0), "MemAvailable") == 0) {
            sm.ram_available_bytes = str2ull(procfile_lineword(ff, line, 1), NULL) * 1024;
            matched_available = true;
        }

        if(matched_total && matched_available)
            break;
    }

    // we keep ff open to speed up the next calls
    return sm;

failed:
    sm.ram_total_bytes = 0;
    sm.ram_available_bytes = 0;
    return sm;
}

typedef enum {
    OS_MEM_SRC_UNKNOWN,
    OS_MEM_SRC_CGROUP_V1,
    OS_MEM_SRC_CGROUP_V2,
    OS_MEM_SRC_MEMINFO,
} OS_MEM_SRC;

OS_SYSTEM_MEMORY os_system_memory(bool query_total_ram __maybe_unused) {
    static OS_SYSTEM_MEMORY sm = {0, 0};
    static usec_t last_ut = 0, last_total_ut = 0;
    static OS_MEM_SRC src = OS_MEM_SRC_UNKNOWN;

    usec_t now_ut = now_monotonic_usec();
    if(sm.ram_total_bytes && sm.ram_available_bytes && last_ut + USEC_PER_MS > now_ut)
        return sm;

    last_ut = now_ut;

    if(query_total_ram)
        // let it auto-detect
        src = OS_MEM_SRC_UNKNOWN;

    if(last_total_ut + USEC_PER_SEC > now_ut)
        // query also the total ram
        query_total_ram = true;

    switch(src) {
        case OS_MEM_SRC_MEMINFO:
            sm = os_system_memory_meminfo(query_total_ram);
            break;

        case OS_MEM_SRC_CGROUP_V2:
            sm = os_system_memory_cgroup_v2(query_total_ram);
            break;

        case OS_MEM_SRC_CGROUP_V1:
            sm = os_system_memory_cgroup_v1(query_total_ram);
            break;

        default:
        case OS_MEM_SRC_UNKNOWN: {
            OS_SYSTEM_MEMORY mi = os_system_memory_meminfo(true);
            OS_SYSTEM_MEMORY v1 = os_system_memory_cgroup_v1(true);
            OS_SYSTEM_MEMORY v2 = os_system_memory_cgroup_v2(true);

            if(v2.ram_total_bytes && v2.ram_available_bytes && v2.ram_total_bytes <= mi.ram_total_bytes && v2.ram_available_bytes < mi.ram_available_bytes) {
                sm = v2;
                src = OS_MEM_SRC_CGROUP_V2;
            }
            if(v1.ram_total_bytes && v1.ram_available_bytes && v1.ram_total_bytes <= mi.ram_total_bytes && v1.ram_available_bytes < mi.ram_available_bytes) {
                sm = v1;
                src = OS_MEM_SRC_CGROUP_V1;
            }
            else {
                sm = mi;
                src = OS_MEM_SRC_MEMINFO;
            }
        }
    }

    return sm;
}
#endif

// FreeBSD
#if defined(OS_FREEBSD)
#include <sys/types.h>
#include <sys/sysctl.h>

OS_SYSTEM_MEMORY os_system_memory(bool query_total_ram) {
    static OS_SYSTEM_MEMORY sm = {0, 0};

    // Query the total RAM only if needed or if it hasn't been cached
    if (query_total_ram || sm.ram_total_bytes == 0) {
        uint64_t total_pages = 0;
        size_t size = sizeof(total_pages);
        if (sysctlbyname("vm.stats.vm.v_page_count", &total_pages, &size, NULL, 0) != 0)
            goto failed;

        unsigned long page_size = 0;
        size = sizeof(page_size);
        if (sysctlbyname("hw.pagesize", &page_size, &size, NULL, 0) != 0)
            goto failed;

        sm.ram_total_bytes = total_pages * page_size;
    }

    // Query the available RAM (free + inactive pages)
    uint64_t free_pages = 0, inactive_pages = 0;
    size_t size = sizeof(free_pages);
    if (sysctlbyname("vm.stats.vm.v_free_count", &free_pages, &size, NULL, 0) != 0 ||
        sysctlbyname("vm.stats.vm.v_inactive_count", &inactive_pages, &size, NULL, 0) != 0)
        goto failed;

    unsigned long page_size = 0;
    size = sizeof(page_size);
    if (sysctlbyname("hw.pagesize", &page_size, &size, NULL, 0) != 0)
        goto failed;

    sm.ram_available_bytes = (free_pages + inactive_pages) * page_size;

    return sm;

failed:
    sm.ram_total_bytes = 0;
    sm.ram_available_bytes = 0;
    return sm;
}
#endif
