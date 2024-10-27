// SPDX-License-Identifier: GPL-3.0-or-later

#include "perflib.h"

#if defined(OS_WINDOWS)
static const char *getCounterType(DWORD CounterType) {
    switch (CounterType) {
        case PERF_COUNTER_COUNTER:
            return "PERF_COUNTER_COUNTER";

        case PERF_COUNTER_TIMER:
            return "PERF_COUNTER_TIMER";

        case PERF_COUNTER_QUEUELEN_TYPE:
            return "PERF_COUNTER_QUEUELEN_TYPE";

        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
            return "PERF_COUNTER_LARGE_QUEUELEN_TYPE";

        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
            return "PERF_COUNTER_100NS_QUEUELEN_TYPE";

        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
            return "PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE";

        case PERF_COUNTER_BULK_COUNT:
            return "PERF_COUNTER_BULK_COUNT";

        case PERF_COUNTER_TEXT:
            return "PERF_COUNTER_TEXT";

        case PERF_COUNTER_RAWCOUNT:
            return "PERF_COUNTER_RAWCOUNT";

        case PERF_COUNTER_LARGE_RAWCOUNT:
            return "PERF_COUNTER_LARGE_RAWCOUNT";

        case PERF_COUNTER_RAWCOUNT_HEX:
            return "PERF_COUNTER_RAWCOUNT_HEX";

        case PERF_COUNTER_LARGE_RAWCOUNT_HEX:
            return "PERF_COUNTER_LARGE_RAWCOUNT_HEX";

        case PERF_SAMPLE_FRACTION:
            return "PERF_SAMPLE_FRACTION";

        case PERF_SAMPLE_COUNTER:
            return "PERF_SAMPLE_COUNTER";

        case PERF_COUNTER_NODATA:
            return "PERF_COUNTER_NODATA";

        case PERF_COUNTER_TIMER_INV:
            return "PERF_COUNTER_TIMER_INV";

        case PERF_SAMPLE_BASE:
            return "PERF_SAMPLE_BASE";

        case PERF_AVERAGE_TIMER:
            return "PERF_AVERAGE_TIMER";

        case PERF_AVERAGE_BASE:
            return "PERF_AVERAGE_BASE";

        case PERF_AVERAGE_BULK:
            return "PERF_AVERAGE_BULK";

        case PERF_OBJ_TIME_TIMER:
            return "PERF_OBJ_TIME_TIMER";

        case PERF_100NSEC_TIMER:
            return "PERF_100NSEC_TIMER";

        case PERF_100NSEC_TIMER_INV:
            return "PERF_100NSEC_TIMER_INV";

        case PERF_COUNTER_MULTI_TIMER:
            return "PERF_COUNTER_MULTI_TIMER";

        case PERF_COUNTER_MULTI_TIMER_INV:
            return "PERF_COUNTER_MULTI_TIMER_INV";

        case PERF_COUNTER_MULTI_BASE:
            return "PERF_COUNTER_MULTI_BASE";

        case PERF_100NSEC_MULTI_TIMER:
            return "PERF_100NSEC_MULTI_TIMER";

        case PERF_100NSEC_MULTI_TIMER_INV:
            return "PERF_100NSEC_MULTI_TIMER_INV";

        case PERF_RAW_FRACTION:
            return "PERF_RAW_FRACTION";

        case PERF_LARGE_RAW_FRACTION:
            return "PERF_LARGE_RAW_FRACTION";

        case PERF_RAW_BASE:
            return "PERF_RAW_BASE";

        case PERF_LARGE_RAW_BASE:
            return "PERF_LARGE_RAW_BASE";

        case PERF_ELAPSED_TIME:
            return "PERF_ELAPSED_TIME";

        case PERF_COUNTER_HISTOGRAM_TYPE:
            return "PERF_COUNTER_HISTOGRAM_TYPE";

        case PERF_COUNTER_DELTA:
            return "PERF_COUNTER_DELTA";

        case PERF_COUNTER_LARGE_DELTA:
            return "PERF_COUNTER_LARGE_DELTA";

        case PERF_PRECISION_SYSTEM_TIMER:
            return "PERF_PRECISION_SYSTEM_TIMER";

        case PERF_PRECISION_100NS_TIMER:
            return "PERF_PRECISION_100NS_TIMER";

        case PERF_PRECISION_OBJECT_TIMER:
            return "PERF_PRECISION_OBJECT_TIMER";

        default:
            return "UNKNOWN_COUNTER_TYPE";
    }
}

static const char *getCounterDescription(DWORD CounterType) {
    switch (CounterType) {
        case PERF_COUNTER_COUNTER:
            return "32-bit Counter. Divide delta by delta time. Display suffix: \"/sec\"";

        case PERF_COUNTER_TIMER:
            return "64-bit Timer. Divide delta by delta time. Display suffix: \"%\"";

        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
            return "Queue Length Space-Time Product. Divide delta by delta time. No Display Suffix";

        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
            return "Queue Length Space-Time Product using 100 Ns timebase. Divide delta by delta time. No Display Suffix";

        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
            return "Queue Length Space-Time Product using Object specific timebase. Divide delta by delta time. No Display Suffix.";

        case PERF_COUNTER_BULK_COUNT:
            return "64-bit Counter.  Divide delta by delta time. Display Suffix: \"/sec\"";

        case PERF_COUNTER_TEXT:
            return "Unicode text Display as text.";

        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_LARGE_RAWCOUNT:
            return "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix.";

        case PERF_COUNTER_RAWCOUNT_HEX:
        case PERF_COUNTER_LARGE_RAWCOUNT_HEX:
            return "Special case for RAWCOUNT which should be displayed in hex. A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix.";

        case PERF_SAMPLE_FRACTION:
            return "A count which is either 1 or 0 on each sampling interrupt (% busy). Divide delta by delta base. Display Suffix: \"%\"";

        case PERF_SAMPLE_COUNTER:
            return "A count which is sampled on each sampling interrupt (queue length). Divide delta by delta time. No Display Suffix.";

        case PERF_COUNTER_NODATA:
            return "A label: no data is associated with this counter (it has 0 length). Do not display.";

        case PERF_COUNTER_TIMER_INV:
            return "64-bit Timer inverse (e.g., idle is measured, but display busy %). Display 100 - delta divided by delta time.  Display suffix: \"%\"";

        case PERF_SAMPLE_BASE:
            return "The divisor for a sample, used with the previous counter to form a sampled %. You must check for >0 before dividing by this! This counter will directly follow the numerator counter. It should not be displayed to the user.";

        case PERF_AVERAGE_TIMER:
            return "A timer which, when divided by an average base, produces a time in seconds which is the average time of some operation. This timer times total operations, and the base is the number of operations. Display Suffix: \"sec\"";

        case PERF_AVERAGE_BASE:
            return "Used as the denominator in the computation of time or count averages. Must directly follow the numerator counter. Not displayed to the user.";

        case PERF_AVERAGE_BULK:
            return "A bulk count which, when divided (typically) by the number of operations, gives (typically) the number of bytes per operation. No Display Suffix.";

        case PERF_OBJ_TIME_TIMER:
            return "64-bit Timer in object specific units. Display delta divided by delta time as returned in the object type header structure.  Display suffix: \"%\"";

        case PERF_100NSEC_TIMER:
            return "64-bit Timer in 100 nsec units. Display delta divided by delta time. Display suffix: \"%\"";

        case PERF_100NSEC_TIMER_INV:
            return "64-bit Timer inverse (e.g., idle is measured, but display busy %). Display 100 - delta divided by delta time.  Display suffix: \"%\"";

        case PERF_COUNTER_MULTI_TIMER:
            return "64-bit Timer.  Divide delta by delta time.  Display suffix: \"%\". Timer for multiple instances, so result can exceed 100%.";

        case PERF_COUNTER_MULTI_TIMER_INV:
            return "64-bit Timer inverse (e.g., idle is measured, but display busy %). Display 100 * _MULTI_BASE - delta divided by delta time. Display suffix: \"%\" Timer for multiple instances, so result can exceed 100%. Followed by a counter of type _MULTI_BASE.";

        case PERF_COUNTER_MULTI_BASE:
            return "Number of instances to which the preceding _MULTI_..._INV counter applies. Used as a factor to get the percentage.";

        case PERF_100NSEC_MULTI_TIMER:
            return "64-bit Timer in 100 nsec units. Display delta divided by delta time. Display suffix: \"%\" Timer for multiple instances, so result can exceed 100%.";

        case PERF_100NSEC_MULTI_TIMER_INV:
            return "64-bit Timer inverse (e.g., idle is measured, but display busy %). Display 100 * _MULTI_BASE - delta divided by delta time. Display suffix: \"%\" Timer for multiple instances, so result can exceed 100%. Followed by a counter of type _MULTI_BASE.";

        case PERF_LARGE_RAW_FRACTION:
        case PERF_RAW_FRACTION:
            return "Indicates the data is a fraction of the following counter  which should not be time averaged on display (such as free space over total space.) Display as is. Display the quotient as \"%\"";

        case PERF_RAW_BASE:
        case PERF_LARGE_RAW_BASE:
            return "Indicates the data is a base for the preceding counter which should not be time averaged on display (such as free space over total space.)";

        case PERF_ELAPSED_TIME:
            return "The data collected in this counter is actually the start time of the item being measured. For display, this data is subtracted from the sample time to yield the elapsed time as the difference between the two. In the definition below, the PerfTime field of the Object contains the sample time as indicated by the PERF_OBJECT_TIMER bit and the difference is scaled by the PerfFreq of the Object to convert the time units into seconds.";

        case PERF_COUNTER_HISTOGRAM_TYPE:
            return "Counter type can be used with the preceding types to define a range of values to be displayed in a histogram.";

        case PERF_COUNTER_DELTA:
        case PERF_COUNTER_LARGE_DELTA:
            return "This counter is used to display the difference from one sample to the next. The counter value is a constantly increasing number  and the value displayed is the difference between the current value and the previous value. Negative numbers are not allowed which shouldn't be a problem as long as the counter value is increasing or unchanged.";

        case PERF_PRECISION_SYSTEM_TIMER:
            return "The precision counters are timers that consist of two counter values:\r\n\t1) the count of elapsed time of the event being monitored\r\n\t2) the \"clock\" time in the same units\r\nthe precision timers are used where the standard system timers are not precise enough for accurate readings. It's assumed that the service providing the data is also providing a timestamp at the same time which will eliminate any error that may occur since some small and variable time elapses between the time the system timestamp is captured and when the data is collected from the performance DLL. Only in extreme cases has this been observed to be problematic.\r\nwhen using this type of timer, the definition of the PERF_PRECISION_TIMESTAMP counter must immediately follow the definition of the PERF_PRECISION_*_TIMER in the Object header\r\nThe timer used has the same frequency as the System Performance Timer";

        case PERF_PRECISION_100NS_TIMER:
            return "The precision counters are timers that consist of two counter values:\r\n\t1) the count of elapsed time of the event being monitored\r\n\t2) the \"clock\" time in the same units\r\nthe precision timers are used where the standard system timers are not precise enough for accurate readings. It's assumed that the service providing the data is also providing a timestamp at the same time which will eliminate any error that may occur since some small and variable time elapses between the time the system timestamp is captured and when the data is collected from the performance DLL. Only in extreme cases has this been observed to be problematic.\r\nwhen using this type of timer, the definition of the PERF_PRECISION_TIMESTAMP counter must immediately follow the definition of the PERF_PRECISION_*_TIMER in the Object header\r\nThe timer used has the same frequency as the 100 NanoSecond Timer";

        case PERF_PRECISION_OBJECT_TIMER:
            return "The precision counters are timers that consist of two counter values:\r\n\t1) the count of elapsed time of the event being monitored\r\n\t2) the \"clock\" time in the same units\r\nthe precision timers are used where the standard system timers are not precise enough for accurate readings. It's assumed that the service providing the data is also providing a timestamp at the same time which will eliminate any error that may occur since some small and variable time elapses between the time the system timestamp is captured and when the data is collected from the performance DLL. Only in extreme cases has this been observed to be problematic.\r\nwhen using this type of timer, the definition of the PERF_PRECISION_TIMESTAMP counter must immediately follow the definition of the PERF_PRECISION_*_TIMER in the Object header\r\nThe timer used is of the frequency specified in the Object header's. PerfFreq field (PerfTime is ignored)";

        default:
            return "";
    }
}

static const char *getCounterAlgorithm(DWORD CounterType) {
    switch (CounterType)
    {
        case PERF_COUNTER_COUNTER:
        case PERF_SAMPLE_COUNTER:
        case PERF_COUNTER_BULK_COUNT:
            return "(data1 - data0) / ((time1 - time0) / frequency)";

        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
        case PERF_AVERAGE_BULK:  // normally not displayed
            return "(data1 - data0) / (time1 - time0)";

        case PERF_OBJ_TIME_TIMER:
        case PERF_COUNTER_TIMER:
        case PERF_100NSEC_TIMER:
        case PERF_PRECISION_SYSTEM_TIMER:
        case PERF_PRECISION_100NS_TIMER:
        case PERF_PRECISION_OBJECT_TIMER:
        case PERF_SAMPLE_FRACTION:
            return "100 * (data1 - data0) / (time1 - time0)";

        case PERF_COUNTER_TIMER_INV:
            return "100 * (1 - ((data1 - data0) / (time1 - time0)))";

        case PERF_100NSEC_TIMER_INV:
            return "100 * (1- (data1 - data0) / (time1 - time0))";

        case PERF_COUNTER_MULTI_TIMER:
            return "100 * ((data1 - data0) / ((time1 - time0) / frequency1)) / multi1";

        case PERF_100NSEC_MULTI_TIMER:
            return "100 * ((data1 - data0) / (time1 - time0)) / multi1";

        case PERF_COUNTER_MULTI_TIMER_INV:
        case PERF_100NSEC_MULTI_TIMER_INV:
            return "100 * (multi1 - ((data1 - data0) / (time1 - time0)))";

        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_LARGE_RAWCOUNT:
            return "data0";

        case PERF_COUNTER_RAWCOUNT_HEX:
        case PERF_COUNTER_LARGE_RAWCOUNT_HEX:
            return "hex(data0)";

        case PERF_COUNTER_DELTA:
        case PERF_COUNTER_LARGE_DELTA:
            return "data1 - data0";

        case PERF_RAW_FRACTION:
        case PERF_LARGE_RAW_FRACTION:
            return "100 * data0 / time0";

        case PERF_AVERAGE_TIMER:
            return "((data1 - data0) / frequency1) / (time1 - time0)";

        case PERF_ELAPSED_TIME:
            return "(time0 - data0) / frequency0";

        case PERF_COUNTER_TEXT:
        case PERF_SAMPLE_BASE:
        case PERF_AVERAGE_BASE:
        case PERF_COUNTER_MULTI_BASE:
        case PERF_RAW_BASE:
        case PERF_COUNTER_NODATA:
        case PERF_PRECISION_TIMESTAMP:
        default:
            return "";
    }
}

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
    char name[4096];
    if(!getSystemName(pDataBlock, name, sizeof(name)))
        strncpyz(name, "[failed]", sizeof(name) - 1);

    BUFFER *wb = data;
    buffer_json_member_add_string(wb, "SystemName", name);

    // Number of types of objects being reported
    // Type: DWORD
    buffer_json_member_add_int64(wb, "NumObjectTypes", pDataBlock->NumObjectTypes);

    buffer_json_member_add_int64(wb, "LittleEndian", pDataBlock->LittleEndian);

    // Version and Revision of these data structures.
    // Version starts at 1.
    // Revision starts at 0 for each Version.
    // Type: DWORD
    buffer_json_member_add_int64(wb, "Version", pDataBlock->Version);
    buffer_json_member_add_int64(wb, "Revision", pDataBlock->Revision);

    // Object Title Index of default object to display when data from this system is retrieved
    // (-1 = none, but this is not expected to be used)
    // Type: LONG
    buffer_json_member_add_int64(wb, "DefaultObject", pDataBlock->DefaultObject);

    // Performance counter frequency at the system under measurement
    // Type: LARGE_INTEGER
    buffer_json_member_add_int64(wb, "PerfFreq", pDataBlock->PerfFreq.QuadPart);

    // Performance counter value at the system under measurement
    // Type: LARGE_INTEGER
    buffer_json_member_add_int64(wb, "PerfTime", pDataBlock->PerfTime.QuadPart);

    // Performance counter time in 100 nsec units at the system under measurement
    // Type: LARGE_INTEGER
    buffer_json_member_add_int64(wb, "PerfTime100nSec", pDataBlock->PerfTime100nSec.QuadPart);

    // Time at the system under measurement in UTC
    // Type: SYSTEMTIME
    buffer_json_member_add_object(wb, "SystemTime");
    dumpSystemTime(wb, &pDataBlock->SystemTime);
    buffer_json_object_close(wb);

    if(pDataBlock->NumObjectTypes)
        buffer_json_member_add_array(wb, "Objects");

    return true;
}

static const char *GetDetailLevel(DWORD num) {
    switch (num) {
        case 100:
            return "Novice (100)";
        case 200:
            return "Advanced (200)";
        case 300:
            return "Expert (300)";
        case 400:
            return "Wizard (400)";

        default:
            return "Unknown";
    }
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
    buffer_json_member_add_string(wb, "DetailLevel", GetDetailLevel(pObjectType->DetailLevel));

    if(ObjectTypeHasInstances(pDataBlock, pObjectType))
        buffer_json_member_add_array(wb, "Instances");
    else
        buffer_json_member_add_array(wb, "Counters");

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
        strncpyz(name, "[failed]", sizeof(name) - 1);

    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "Instance", name);
    buffer_json_member_add_int64(wb, "UniqueID", pInstance->UniqueID);
    buffer_json_member_add_array(wb, "Labels");
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
                    strncpyz(name, "[failed]", sizeof(name) - 1);

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

    buffer_json_member_add_array(wb, "Counters");
    return true;
}

void dumpSample(BUFFER *wb, RAW_DATA *d) {
    buffer_json_member_add_object(wb, "Value");
    buffer_json_member_add_uint64(wb, "data", d->Data);
    buffer_json_member_add_int64(wb, "time", d->Time);
    buffer_json_member_add_uint64(wb, "type", d->CounterType);
    buffer_json_member_add_int64(wb, "multi", d->MultiCounterData);
    buffer_json_member_add_int64(wb, "frequency", d->Frequency);
    buffer_json_object_close(wb);
}

bool dumpCounterCb(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_COUNTER_DEFINITION *pCounter, RAW_DATA *sample, void *data) {
    (void)pDataBlock;
    (void)pObjectType;
    BUFFER *wb = data;
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "Counter", RegistryFindNameByID(pCounter->CounterNameTitleIndex));
    dumpSample(wb, sample);
    buffer_json_member_add_string(wb, "Help", RegistryFindHelpByID(pCounter->CounterHelpTitleIndex));
    buffer_json_member_add_string(wb, "Type", getCounterType(pCounter->CounterType));
    buffer_json_member_add_string(wb, "Algorithm", getCounterAlgorithm(pCounter->CounterType));
    buffer_json_member_add_string(wb, "Description", getCounterDescription(pCounter->CounterType));
    buffer_json_object_close(wb);
    return true;
}

bool dumpInstanceCounterCb(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, PERF_COUNTER_DEFINITION *pCounter, RAW_DATA *sample, void *data) {
    (void)pInstance;
    return dumpCounterCb(pDataBlock, pObjectType, pCounter, sample, data);
}


int windows_perflib_dump(const char *key) {
    if(key && !*key)
        key = NULL;

    PerflibNamesRegistryInitialize();

    DWORD id = 0;
    if(key) {
        id = RegistryFindIDByName(key);
        if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND) {
            fprintf(stderr, "Cannot find key '%s' in Windows Performance Counters Registry.\n", key);
            exit(1);
        }
    }

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    perflibQueryAndTraverse(id, dumpDataCb, dumpObjectCb, dumpInstanceCb, dumpInstanceCounterCb, dumpCounterCb, wb);

    buffer_json_finalize(wb);
    printf("\n%s\n", buffer_tostring(wb));

    perflibFreePerformanceData();

    return 0;
}

#endif // OS_WINDOWS