// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef PCF_HELPERS_H
#define PCF_HELPERS_H

#include <cmqc.h>
#include <cmqxc.h>
#include <cmqcfc.h>
#include <string.h>
#include <stdlib.h>

// Helper functions to work around CGO type issues
static inline void set_object_name(MQOD* od, const char* name) {
    // MQ expects names to be padded with spaces, not nulls
    memset(od->ObjectName, ' ', sizeof(od->ObjectName));
    size_t len = strlen(name);
    if (len > sizeof(od->ObjectName)) {
        len = sizeof(od->ObjectName);
    }
    memcpy(od->ObjectName, name, len);
}

static inline void set_format(MQMD* md, const char* format) {
    // MQ expects format to be exactly 8 chars, padded with spaces
    memset(md->Format, ' ', sizeof(md->Format));
    size_t len = strlen(format);
    if (len > sizeof(md->Format)) {
        len = sizeof(md->Format);
    }
    memcpy(md->Format, format, len);
}

static inline void copy_msg_id(MQMD* dest, MQMD* src) {
    memcpy(dest->CorrelId, src->MsgId, sizeof(src->MsgId));
}

static inline void set_csp_struc_id(MQCSP* csp) {
    memcpy(csp->StrucId, MQCSP_STRUC_ID, sizeof(csp->StrucId));
}

static inline void set_cno_struc_id(MQCNO* cno) {
    memcpy(cno->StrucId, MQCNO_STRUC_ID, sizeof(cno->StrucId));
}

static inline void set_od_struc_id(MQOD* od) {
    memcpy(od->StrucId, MQOD_STRUC_ID, sizeof(od->StrucId));
}

static inline void set_md_struc_id(MQMD* md) {
    memcpy(md->StrucId, MQMD_STRUC_ID, sizeof(md->StrucId));
}

static inline void set_pmo_struc_id(MQPMO* pmo) {
    memcpy(pmo->StrucId, MQPMO_STRUC_ID, sizeof(pmo->StrucId));
}

static inline void set_gmo_struc_id(MQGMO* gmo) {
    memcpy(gmo->StrucId, MQGMO_STRUC_ID, sizeof(gmo->StrucId));
}

#endif // PCF_HELPERS_H