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

static inline void set_dynamic_q_name(MQOD* od, const char* name) {
    // Dynamic queue name pattern
    memset(od->DynamicQName, ' ', sizeof(od->DynamicQName));
    size_t len = strlen(name);
    if (len > sizeof(od->DynamicQName)) {
        len = sizeof(od->DynamicQName);
    }
    memcpy(od->DynamicQName, name, len);
}

static inline void set_sd_struc_id(MQSD* sd) {
    memcpy(sd->StrucId, MQSD_STRUC_ID, sizeof(sd->StrucId));
}

static inline void set_sub_name(MQSD* sd, const char* name) {
    // SubName is an MQCHARV structure, not a simple char array
    // Set the string pointer and length
    sd->SubName.VSPtr = (MQPTR)name;
    sd->SubName.VSLength = (MQLONG)strlen(name);
}

static inline void set_topic_string(MQSD* sd, const char* topic) {
    // ObjectString is an MQCHARV structure for topic strings
    sd->ObjectString.VSPtr = (MQPTR)topic;
    sd->ObjectString.VSLength = (MQLONG)strlen(topic);
}

static inline void init_mqsd(MQSD* sd) {
    memset(sd, 0, sizeof(MQSD));
    set_sd_struc_id(sd);
    sd->Version = MQSD_VERSION_1;
}

static inline void init_mqmd(MQMD* md) {
    memset(md, 0, sizeof(MQMD));
    set_md_struc_id(md);
    md->Version = MQMD_VERSION_1;
}

static inline void init_mqgmo(MQGMO* gmo) {
    memset(gmo, 0, sizeof(MQGMO));
    set_gmo_struc_id(gmo);
    gmo->Version = MQGMO_VERSION_1;
}

#endif // PCF_HELPERS_H