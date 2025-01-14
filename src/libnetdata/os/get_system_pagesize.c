// SPDX-License-Identifier: GPL-3.0-or-later

#include "get_system_pagesize.h"

size_t os_get_system_page_size(void) {
    static long int page_size = 0;

    if(unlikely(!page_size)) {
        page_size = sysconf(_SC_PAGE_SIZE);
        if (unlikely(page_size <= 4096))
            page_size = 4096;
    }

    return page_size;
}
