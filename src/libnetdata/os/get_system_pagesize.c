// SPDX-License-Identifier: GPL-3.0-or-later

#include "get_system_pagesize.h"

#if defined(OS_WINDOWS)
#include <windows.h>

size_t os_get_system_page_size(void) {
    static size_t page_size = 0;

    if (!page_size) {
        SYSTEM_INFO sys_info;
        GetSystemInfo(&sys_info);

        // Set the page size, default to 4096 if for some reason it's invalid
        page_size = (sys_info.dwPageSize >= 4096) ? sys_info.dwPageSize : 4096;
    }

    return page_size;
}

#else

size_t os_get_system_page_size(void) {
    static long int page_size = 0;

    if(unlikely(!page_size)) {
        page_size = sysconf(_SC_PAGE_SIZE);
        if (unlikely(page_size <= 4096))
            page_size = 4096;
    }

    return page_size;
}

#endif