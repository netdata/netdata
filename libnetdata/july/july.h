// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JULY_H
#define NETDATA_JULY_H 1

#include "../libnetdata.h"

// #define PDC_USE_JULYL 1

PPvoid_t JulyLGet(Pcvoid_t PArray, Word_t Index, PJError_t PJError);
PPvoid_t JulyLIns(PPvoid_t PPArray, Word_t Index, PJError_t PJError);
PPvoid_t JulyLFirst(Pcvoid_t PArray, Word_t *Index, PJError_t PJError);
PPvoid_t JulyLNext(Pcvoid_t PArray, Word_t *Index, PJError_t PJError);
PPvoid_t JulyLLast(Pcvoid_t PArray, Word_t *Index, PJError_t PJError);
PPvoid_t JulyLPrev(Pcvoid_t PArray, Word_t *Index, PJError_t PJError);
Word_t JulyLFreeArray(PPvoid_t PPArray, PJError_t PJError);

static inline PPvoid_t JulyLFirstThenNext(Pcvoid_t PArray, Word_t * PIndex, bool *first) {
    if(unlikely(*first)) {
        *first = false;
        return JulyLFirst(PArray, PIndex, PJE0);
    }

    return JulyLNext(PArray, PIndex, PJE0);
}

static inline PPvoid_t JulyLLastThenPrev(Pcvoid_t PArray, Word_t * PIndex, bool *first) {
    if(unlikely(*first)) {
        *first = false;
        return JulyLLast(PArray, PIndex, PJE0);
    }

    return JulyLPrev(PArray, PIndex, PJE0);
}

void julyl_cleanup1(void);
size_t julyl_cache_size(void);
size_t julyl_bytes_moved(void);

#endif // NETDATA_JULY_H
