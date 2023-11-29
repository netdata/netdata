// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void duplication_cleanup(DUPLICATION *dp) {
    if(dp->target)
        freez(dp->target);

    for(size_t j = 0; j < dp->used ; j++) {
        if (dp->keys[j])
            freez(dp->keys[j]);

        if (dp->values[j].s)
            freez(dp->values[j].s);
    }
}