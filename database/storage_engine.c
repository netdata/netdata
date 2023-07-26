// SPDX-License-Identifier: GPL-3.0-or-later

#include "storage_engine.h"

#include "ram/rrddim_mem.h"
#ifdef ENABLE_DBENGINE
#include "engine/rrdengineapi.h"
#endif

#ifdef ENABLE_DBENGINE
STORAGE_ENGINE_ID default_storage_engine_id = STORAGE_ENGINE_DBENGINE;
#else
STORAGE_ENGINE_ID default_storage_engine_id = STORAGE_ENGINE_SAVE;
#endif

const char *storage_engine_name(STORAGE_ENGINE_ID id) {
    switch(id) {
        case STORAGE_ENGINE_RAM:
            return STORAGE_ENGINE_RAM_NAME;

        case STORAGE_ENGINE_MAP:
            return STORAGE_ENGINE_MAP_NAME;

        case STORAGE_ENGINE_NONE:
            return STORAGE_ENGINE_NONE_NAME;

        case STORAGE_ENGINE_SAVE:
            return STORAGE_ENGINE_SAVE_NAME;

        case STORAGE_ENGINE_ALLOC:
            return STORAGE_ENGINE_ALLOC_NAME;

        case STORAGE_ENGINE_DBENGINE:
            return STORAGE_ENGINE_DBENGINE_NAME;

        default:
            __builtin_unreachable();
    }
}

bool storage_engine_id(const char *name, STORAGE_ENGINE_ID *id) {
    if (!strcmp(name, STORAGE_ENGINE_RAM_NAME)) {
        *id = STORAGE_ENGINE_RAM;
    } else if (!strcmp(name, STORAGE_ENGINE_MAP_NAME)) {
        *id = STORAGE_ENGINE_MAP;
    } else if (!strcmp(name, STORAGE_ENGINE_NONE_NAME)) {
        *id = STORAGE_ENGINE_NONE;
    } else if (!strcmp(name, STORAGE_ENGINE_SAVE_NAME)) {
        *id = STORAGE_ENGINE_SAVE;
    } else if (!strcmp(name, STORAGE_ENGINE_ALLOC_NAME)) {
        *id = STORAGE_ENGINE_ALLOC;
    } else if (!strcmp(name, STORAGE_ENGINE_DBENGINE_NAME)) {
        *id = STORAGE_ENGINE_DBENGINE;
    } else {
        return false;
    }

    return true;
}
