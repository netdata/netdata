
#include "rrd.h"

inline const char *rrd_memory_mode_name(RRD_DB_MODE id) {
    switch(id) {
        case RRD_DB_MODE_RAM:
            return RRD_DB_MODE_RAM_NAME;

        case RRD_DB_MODE_NONE:
            return RRD_DB_MODE_NONE_NAME;

        case RRD_DB_MODE_ALLOC:
            return RRD_DB_MODE_ALLOC_NAME;

        case RRD_DB_MODE_DBENGINE:
            return RRD_DB_MODE_DBENGINE_NAME;
    }

    STORAGE_ENGINE* eng = storage_engine_get(id);
    if (eng) {
        return eng->name;
    }

    return RRD_DB_MODE_RAM_NAME;
}

RRD_DB_MODE rrd_memory_mode_id(const char *name) {
    STORAGE_ENGINE* eng = storage_engine_find(name);
    if (eng) {
        return eng->id;
    }

    return RRD_DB_MODE_RAM;
}
