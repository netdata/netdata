// SPDX-License-Identifier: GPL-3.0-or-later

#include "perflib.h"
#include "windows-internals.h"


void dumpSystemTime(BUFFER *wb, SYSTEMTIME *st) {
    buffer_json_member_add_uint64(wb, "Year", st->wYear);
    buffer_json_member_add_uint64(wb, "Month", st->wMonth);
    buffer_json_member_add_uint64(wb, "DayOfWeek", st->wDayOfWeek);
    buffer_json_member_add_uint64(wb, "Day", st->wDay);
    buffer_json_member_add_uint64(wb, "Hour", st->wHour);
    buffer_json_member_add_uint64(wb, "Minute", st->wMinute);
    buffer_json_member_add_uint64(wb, "Second", st->wSecond);
    buffer_json_member_add_uint64(wb, "Milliseconds", st->wMilliseconds);
}

bool dumpDataCb(PERF_DATA_BLOCK *pDataBlock, void *data) {
    BUFFER *wb = data;
    buffer_json_member_add_string(wb, "SystemName", "[unparsed]");
    buffer_json_member_add_int64(wb, "NumObjectTypes", pDataBlock->NumObjectTypes);
    buffer_json_member_add_int64(wb, "LittleEndian", pDataBlock->LittleEndian);
    buffer_json_member_add_int64(wb, "Version", pDataBlock->Version);
    buffer_json_member_add_int64(wb, "Revision", pDataBlock->Revision);
    buffer_json_member_add_int64(wb, "DefaultObject", pDataBlock->DefaultObject);
    buffer_json_member_add_int64(wb, "PerfFreq", pDataBlock->PerfFreq.QuadPart);
    buffer_json_member_add_int64(wb, "PerfTime", pDataBlock->PerfTime.QuadPart);
    buffer_json_member_add_int64(wb, "PerfTime100nSec", pDataBlock->PerfTime100nSec.QuadPart);

    buffer_json_member_add_object(wb, "SystemTime");
    dumpSystemTime(wb, &pDataBlock->SystemTime);
    buffer_json_object_close(wb);

    if(pDataBlock->NumObjectTypes)
        buffer_json_member_add_array(wb, "objects");

    return true;
}

bool dumpObjectCb(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, void *data) {
    (void)pDataBlock;
    BUFFER *wb = data;
    if(!pObjectType) {
        buffer_json_array_close(wb); // instances or counters
        buffer_json_object_close(wb); // objectType
        return true;
    }

    buffer_json_add_array_item_object(wb); // objectType
    buffer_json_member_add_int64(wb, "NameId", pObjectType->ObjectNameTitleIndex);
    buffer_json_member_add_string(wb, "Name", RegistryFindNameByID(pObjectType->ObjectNameTitleIndex));
    buffer_json_member_add_int64(wb, "HelpId", pObjectType->ObjectHelpTitleIndex);
    buffer_json_member_add_string(wb, "Help", RegistryFindHelpByID(pObjectType->ObjectHelpTitleIndex));
    buffer_json_member_add_int64(wb, "NumInstances", pObjectType->NumInstances);
    buffer_json_member_add_int64(wb, "NumCounters", pObjectType->NumCounters);
    buffer_json_member_add_int64(wb, "PerfTime", pObjectType->PerfTime.QuadPart);
    buffer_json_member_add_int64(wb, "PerfFreq", pObjectType->PerfFreq.QuadPart);
    buffer_json_member_add_int64(wb, "CodePage", pObjectType->CodePage);
    buffer_json_member_add_int64(wb, "DefaultCounter", pObjectType->DefaultCounter);
    buffer_json_member_add_int64(wb, "DetailLevel", pObjectType->DetailLevel);

    if(ObjectTypeHasInstances(pDataBlock, pObjectType))
        buffer_json_member_add_array(wb, "instances");
    else
        buffer_json_member_add_array(wb, "counters");

    return true;
}

bool dumpInstanceCb(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, void *data) {
    (void)pDataBlock;
    BUFFER *wb = data;
    if(!pInstance) {
        buffer_json_array_close(wb); // counters
        buffer_json_object_close(wb); // instance
        return true;
    }

    char name[4096];
    if(!getInstanceName(pDataBlock, pObjectType, pInstance, name, sizeof(name)))
        strncpy(name, "[failed]", sizeof(name));

    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_int64(wb, "UniqueID", pInstance->UniqueID);
    buffer_json_member_add_array(wb, "rrdlabels");
    {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "key", RegistryFindNameByID(pObjectType->ObjectNameTitleIndex));
            buffer_json_member_add_string(wb, "value", name);
        }
        buffer_json_object_close(wb);

        if(pInstance->ParentObjectTitleIndex) {
            PERF_INSTANCE_DEFINITION *pi = pInstance;
            while(pi->ParentObjectTitleIndex) {
                PERF_OBJECT_TYPE *po = getObjectTypeByIndex(pDataBlock, pInstance->ParentObjectTitleIndex);
                pi = getInstanceByPosition(pDataBlock, po, pi->ParentObjectInstance);

                if(!getInstanceName(pDataBlock, po, pi, name, sizeof(name)))
                    strncpy(name, "[failed]", sizeof(name));

                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "key", RegistryFindNameByID(po->ObjectNameTitleIndex));
                    buffer_json_member_add_string(wb, "value", name);
                }
                buffer_json_object_close(wb);
            }
        }
    }
    buffer_json_array_close(wb); // rrdlabels

    buffer_json_member_add_array(wb, "counters");
    return true;
}

void dumpSample(BUFFER *wb, RAW_DATA *d) {
    buffer_json_member_add_object(wb, "value");
    buffer_json_member_add_uint64(wb, "data", d->Data);
    buffer_json_member_add_int64(wb, "time", d->Time);
    buffer_json_member_add_uint64(wb, "type", d->CounterType);
    buffer_json_member_add_int64(wb, "frequency", d->Frequency);
    buffer_json_object_close(wb);
}

bool dumpInstanceCounterCb(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, PERF_COUNTER_DEFINITION *pCounter, RAW_DATA *sample, void *data) {
    (void)pDataBlock;
    (void)pObjectType;
    (void)pInstance;
    BUFFER *wb = data;
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "name", RegistryFindNameByID(pCounter->CounterNameTitleIndex));
    dumpSample(wb, sample);
    buffer_json_member_add_string(wb, "help", RegistryFindHelpByID(pCounter->CounterHelpTitleIndex));
    buffer_json_object_close(wb);
    return true;
}

bool dumpCounterCb(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_COUNTER_DEFINITION *pCounter, RAW_DATA *sample, void *data) {
    (void)pDataBlock;
    (void)pObjectType;
    BUFFER *wb = data;
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "name", RegistryFindNameByID(pCounter->CounterNameTitleIndex));
    dumpSample(wb, sample);
    buffer_json_member_add_string(wb, "help", RegistryFindHelpByID(pCounter->CounterHelpTitleIndex));
    buffer_json_object_close(wb);
    return true;
}

int windows_perflib_dump(void) {
    RegistryInitialize();

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    perflib_query_and_traverse(0, dumpDataCb, dumpObjectCb, dumpInstanceCb, dumpInstanceCounterCb, dumpCounterCb, wb);

    buffer_json_finalize(wb);
    printf("\n%s\n", buffer_tostring(wb));
    return 0;
}
