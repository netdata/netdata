// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// Windows
#if defined(OS_WINDOWS)
#include <windows.h>
uint64_t os_mem_available(void) {
    MEMORYSTATUSEX statex;
    statex.dwLength = sizeof(statex);
    if (GlobalMemoryStatusEx(&statex))
        return statex.ullAvailPhys;

    return 0;
}
#endif

// macOS
#if defined(OS_MACOS)
#include <mach/mach.h>
#include <sys/sysctl.h>
uint64_t os_mem_available(void) {
    static uint64_t page_size = 0;
    if (page_size == 0) {
        size_t len = sizeof(page_size);
        sysctlbyname("hw.pagesize", &page_size, &len, NULL, 0);
    }

    vm_statistics_data_t vm_info;
    mach_msg_type_number_t count = HOST_VM_INFO_COUNT;
    mach_port_t mach_port = mach_host_self();

    if (host_statistics(mach_port, HOST_VM_INFO, (host_info_t)&vm_info, &count) == KERN_SUCCESS) {
        mach_port_deallocate(mach_task_self(), mach_port);
        return (uint64_t)vm_info.free_count * page_size;
    }

    mach_port_deallocate(mach_task_self(), mach_port);
    return 0;
}
#endif

// Linux
#if defined(OS_LINUX)
#if defined(HAVE_SYSINFO)
#include <linux/sysinfo.h>
#include <sys/sysinfo.h>
uint64_t os_mem_available(void) {
    struct sysinfo info;
    if (sysinfo(&info) == 0)
        return (uint64_t)info.freeram * info.mem_unit;

    return 0;
}
#else
uint64_t os_mem_available(void) {
    static uint64_t last_mem_available = 0;
    static usec_t last_ut = 0;
    usec_t now_ut = now_monotonic_usec();
    if(last_ut + 1 * USEC_PER_MS > now_ut)
        return last_mem_available;

    last_ut = now_ut;

    static procfile *ff = NULL;
    if(unlikely(!ff)) {
        ff = procfile_open("/proc/meminfo", ": \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) {
            last_mem_available = 0;
            return 0;
        }
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) {
        last_mem_available = 0;
        return 0;
    }

    size_t lines = procfile_lines(ff);
    for(size_t line = 0; line < lines ;line++) {
        if(strcmp(procfile_lineword(ff, line, 0), "MemAvailable") == 0) {
            last_mem_available = str2ull(procfile_lineword(ff, line, 1), NULL);
            return last_mem_available;
        }
    }

    last_mem_available = 0;
    return 0;
}
#endif
#endif

// FreeBSD
#if defined(OS_FREEBSD)
#include <sys/types.h>
#include <sys/sysctl.h>
uint64_t os_mem_available(void) {
    static unsigned long page_size = 0;
    if (page_size == 0) {
        size_t size = sizeof(page_size);
        sysctlbyname("hw.pagesize", &page_size, &size, NULL, 0);
    }

    int mib[] = {CTL_VM, VM_FREE_COUNT};
    uint64_t free_pages = 0;
    size_t size = sizeof(free_pages);

    if (sysctl(mib, 2, &free_pages, &size, NULL, 0) == 0) {
        return free_pages * page_size;
    }
    return 0;
}
#endif
