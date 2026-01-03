// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "PerflibHyperV"
#include "../common-contexts/common-contexts.h"

#define HYPERV "hyperv"

static void get_and_sanitize_instance_value(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    char *buffer,
    size_t buffer_size)
{
    // char wstr[8192];
    if (!getInstanceName(pDataBlock, pObjectType, pi, buffer, buffer_size)) {
        strncpyz(buffer, "[unknown]", buffer_size - 1);
        // return;
    }
    // rrdlabels_sanitize_value(buffer, wstr, buffer_size);
}

#define DICT_PERF_OPTION (DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE)

#define DEFINE_RD(counter_name) RRDDIM(*rd_##counter_name)

#define GET_INSTANCE_COUNTER(counter)                                                                                  \
    do {                                                                                                               \
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->counter);                                           \
    } while (0)

#define GET_OBJECT_COUNTER(counter)                                                                                    \
    do {                                                                                                               \
        perflibGetObjectCounter(pDataBlock, pObjectType, &p->counter);                                                 \
    } while (0)

#define SETP_DIM_VALUE(st, field)                                                                                      \
    do {                                                                                                               \
        rrddim_set_by_pointer(p->st, p->rd_##field, (collected_number)p->field.current.Data);                          \
    } while (0)

typedef bool (*perf_func_collect)(PERF_DATA_BLOCK *pDataBlock, int update_every, void *data);

typedef struct {
    const char *registry_name;
    perf_func_collect function_collect;
    dict_cb_insert_t dict_insert_cb;
    size_t dict_size;
    DICTIONARY *instance;
} hyperv_perf_item;

struct hypervisor_memory {
    bool collected_metadata;
    bool charts_created;

    RRDSET *st_pressure;
    RRDSET *st_vm_memory_physical;
    RRDSET *st_vm_memory_physical_guest_visible;

    DEFINE_RD(CurrentPressure);
    DEFINE_RD(PhysicalMemory);
    DEFINE_RD(GuestVisiblePhysicalMemory);
    DEFINE_RD(GuestAvailableMemory);

    COUNTER_DATA CurrentPressure;
    COUNTER_DATA PhysicalMemory;
    COUNTER_DATA GuestVisiblePhysicalMemory;
    COUNTER_DATA GuestAvailableMemory;
};

void initialize_hyperv_memory_keys(struct hypervisor_memory *p)
{
    p->CurrentPressure.key = "Current Pressure";
    p->PhysicalMemory.key = "Physical Memory";
    p->GuestVisiblePhysicalMemory.key = "Guest Visible Physical Memory";
    p->GuestAvailableMemory.key = "Guest Available Memory";
}

void dict_hyperv_memory_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct hypervisor_memory *p = value;
    initialize_hyperv_memory_keys(p);
}

struct hypervisor_partition {
    bool collected_metadata;
    bool charts_created;

    RRDSET *st_vm_vid_physical_pages_allocated;
    RRDSET *st_vm_vid_remote_physical_pages;

    DEFINE_RD(PhysicalPagesAllocated);
    DEFINE_RD(RemotePhysicalPages);

    COUNTER_DATA PhysicalPagesAllocated;
    COUNTER_DATA RemotePhysicalPages;
};

void initialize_hyperv_partition_keys(struct hypervisor_partition *p)
{
    p->PhysicalPagesAllocated.key = "Physical Pages Allocated";
    p->RemotePhysicalPages.key = "Remote Physical Pages";
}

void dict_hyperv_partition_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct hypervisor_partition *p = value;
    initialize_hyperv_partition_keys(p);
}

static bool do_hyperv_memory(PERF_DATA_BLOCK *pDataBlock, int update_every, void *data)
{
    hyperv_perf_item *item = data;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, item->registry_name);
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        get_and_sanitize_instance_value(
            pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer));

        struct hypervisor_memory *p = dictionary_set(item->instance, windows_shared_buffer, NULL, sizeof(*p));

        if (!p->collected_metadata) {
            p->collected_metadata = true;
        }

        GET_INSTANCE_COUNTER(CurrentPressure);
        GET_INSTANCE_COUNTER(PhysicalMemory);
        GET_INSTANCE_COUNTER(GuestVisiblePhysicalMemory);
        GET_INSTANCE_COUNTER(GuestAvailableMemory);

        if (!p->charts_created) {
            p->charts_created = true;
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s", windows_shared_buffer);
            netdata_fix_chart_name(id);

            if (!p->st_vm_memory_physical) {
                p->st_vm_memory_physical = rrdset_create_localhost(
                    "vm_memory_physical",
                    id,
                    NULL,
                    HYPERV,
                    HYPERV ".vm_memory_physical",
                    "VM assigned memory",
                    "bytes",
                    _COMMON_PLUGIN_NAME,
                    _COMMON_PLUGIN_MODULE_NAME,
                    NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_MEMORY_PHYSICAL,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->st_vm_memory_physical_guest_visible = rrdset_create_localhost(
                    "vm_memory_physical_guest_visible",
                    windows_shared_buffer,
                    NULL,
                    HYPERV,
                    HYPERV ".vm_memory_physical_guest_visible",
                    "VM guest visible memory",
                    "bytes",
                    _COMMON_PLUGIN_NAME,
                    _COMMON_PLUGIN_MODULE_NAME,
                    NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_MEMORY_PHYSICAL_GUEST_VISIBLE,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->st_pressure = rrdset_create_localhost(
                    "vm_memory_pressure_current",
                    windows_shared_buffer,
                    NULL,
                    HYPERV,
                    HYPERV ".vm_memory_pressure_current",
                    "VM Memory Pressure",
                    "percentage",
                    _COMMON_PLUGIN_NAME,
                    _COMMON_PLUGIN_MODULE_NAME,
                    NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_MEMORY_PRESSURE_CURRENT,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_CurrentPressure = rrddim_add(p->st_pressure, "pressure", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p->rd_PhysicalMemory =
                    rrddim_add(p->st_vm_memory_physical, "assigned", NULL, 1024 * 1024, 1, RRD_ALGORITHM_ABSOLUTE);
                p->rd_GuestVisiblePhysicalMemory = rrddim_add(
                    p->st_vm_memory_physical_guest_visible, "visible", NULL, 1024 * 1024, 1, RRD_ALGORITHM_ABSOLUTE);
                p->rd_GuestAvailableMemory = rrddim_add(
                    p->st_vm_memory_physical_guest_visible, "available", NULL, 1024 * 1024, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(p->st_vm_memory_physical->rrdlabels, "vm_name", windows_shared_buffer, RRDLABEL_SRC_AUTO);
                rrdlabels_add(p->st_pressure->rrdlabels, "vm_name", windows_shared_buffer, RRDLABEL_SRC_AUTO);
                rrdlabels_add(
                    p->st_vm_memory_physical_guest_visible->rrdlabels,
                    "vm_name",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }
        }

        SETP_DIM_VALUE(st_pressure, CurrentPressure);
        SETP_DIM_VALUE(st_vm_memory_physical, PhysicalMemory);
        SETP_DIM_VALUE(st_vm_memory_physical_guest_visible, GuestVisiblePhysicalMemory);
        SETP_DIM_VALUE(st_vm_memory_physical_guest_visible, GuestAvailableMemory);

        rrdset_done(p->st_pressure);
        rrdset_done(p->st_vm_memory_physical);
        rrdset_done(p->st_vm_memory_physical_guest_visible);
    }

    return true;
}

static bool do_hyperv_vid_partition(PERF_DATA_BLOCK *pDataBlock, int update_every, void *data)
{
    hyperv_perf_item *item = data;
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, item->registry_name);
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        get_and_sanitize_instance_value(
            pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer));

        struct hypervisor_partition *p = dictionary_set(item->instance, windows_shared_buffer, NULL, sizeof(*p));

        if (!p->collected_metadata) {
            p->collected_metadata = true;
        }

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        GET_INSTANCE_COUNTER(RemotePhysicalPages);
        GET_INSTANCE_COUNTER(PhysicalPagesAllocated);

        if (!p->charts_created) {
            p->charts_created = true;
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s", windows_shared_buffer);
            netdata_fix_chart_name(id);

            p->st_vm_vid_physical_pages_allocated = rrdset_create_localhost(
                "vm_vid_physical_pages_allocated",
                id,
                NULL,
                HYPERV,
                HYPERV ".vm_vid_physical_pages_allocated",
                "VM physical pages allocated",
                "pages",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_VID_PHYSICAL_PAGES_ALLOCATED,
                update_every,
                RRDSET_TYPE_LINE);

            p->st_vm_vid_remote_physical_pages = rrdset_create_localhost(
                "vm_vid_remote_physical_pages",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vm_vid_remote_physical_pages",
                "VM physical pages not allocated from the preferred NUMA node",
                "pages",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_VID_REMOTE_PHYSICAL_PAGES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_PhysicalPagesAllocated =
                rrddim_add(p->st_vm_vid_physical_pages_allocated, "allocated", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            p->rd_RemotePhysicalPages =
                rrddim_add(p->st_vm_vid_remote_physical_pages, "remote_physical", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(
                p->st_vm_vid_physical_pages_allocated->rrdlabels, "vm_name", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            rrdlabels_add(
                p->st_vm_vid_remote_physical_pages->rrdlabels, "vm_name", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        SETP_DIM_VALUE(st_vm_vid_remote_physical_pages, RemotePhysicalPages);
        SETP_DIM_VALUE(st_vm_vid_physical_pages_allocated, PhysicalPagesAllocated);

        rrdset_done(p->st_vm_vid_physical_pages_allocated);
        rrdset_done(p->st_vm_vid_remote_physical_pages);
    }

    return true;
}

// Define structure for Hyper-V Virtual Machine Health Summary
static struct hypervisor_health_summary {
    bool collected_metadata;
    bool charts_created;

    RRDSET *st_health;

    DEFINE_RD(HealthCritical);
    DEFINE_RD(HealthOk);

    COUNTER_DATA HealthCritical;
    COUNTER_DATA HealthOk;
} health_summary = {
    .collected_metadata = false,
    .st_health = NULL,
    .HealthCritical.key = "Health Critical",
    .HealthOk.key = "Health Ok"};

// Function to handle "Hyper-V Virtual Machine Health Summary"
static bool do_hyperv_health_summary(PERF_DATA_BLOCK *pDataBlock, int update_every, void *data)
{
    hyperv_perf_item *item = data;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, item->registry_name);
    if (!pObjectType)
        return false;

    struct hypervisor_health_summary *p = &health_summary;

    GET_OBJECT_COUNTER(HealthCritical);
    GET_OBJECT_COUNTER(HealthOk);

    if (!p->charts_created) {
        p->charts_created = true;
        p->st_health = rrdset_create_localhost(
            "vms_health",
            "hyperv_health_status",
            NULL,
            HYPERV,
            HYPERV ".vms_health",
            "Virtual machines health status",
            "vms",
            _COMMON_PLUGIN_NAME,
            _COMMON_PLUGIN_MODULE_NAME,
            NETDATA_CHART_PRIO_WINDOWS_HYPERV_VMS_HEALTH,
            update_every,
            RRDSET_TYPE_STACKED);

        p->rd_HealthOk = rrddim_add(p->st_health, "ok", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        p->rd_HealthCritical = rrddim_add(p->st_health, "critical", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    SETP_DIM_VALUE(st_health, HealthOk);
    SETP_DIM_VALUE(st_health, HealthCritical);

    rrdset_done(p->st_health);
    return true;
}

// Define structure for Hyper-V Root Partition Metrics (Device and GPA Space Pages)
struct hypervisor_root_partition {
    bool collected_metadata;
    bool charts_created;

    RRDSET *st_device_space_pages;
    RRDSET *st_gpa_space_pages;
    RRDSET *st_gpa_space_modifications;
    RRDSET *st_attached_devices;
    RRDSET *st_deposited_pages;

    RRDSET *st_DeviceDMAErrors;
    RRDSET *st_DeviceInterruptErrors;
    RRDSET *st_DeviceInterruptThrottleEvents;
    RRDSET *st_IOTLBFlushesSec;
    RRDSET *st_AddressSpaces;
    RRDSET *st_VirtualTLBPages;
    RRDSET *st_VirtualTLBFlushEntiresSec;

    DEFINE_RD(DeviceSpacePages4K);
    DEFINE_RD(DeviceSpacePages2M);
    DEFINE_RD(DeviceSpacePages1G);
    DEFINE_RD(GPASpacePages4K);
    DEFINE_RD(GPASpacePages2M);
    DEFINE_RD(GPASpacePages1G);
    DEFINE_RD(GPASpaceModifications);

    DEFINE_RD(AttachedDevices);
    DEFINE_RD(DepositedPages);

    DEFINE_RD(DeviceDMAErrors);
    DEFINE_RD(DeviceInterruptErrors);
    DEFINE_RD(DeviceInterruptThrottleEvents);
    DEFINE_RD(IOTLBFlushesSec);
    DEFINE_RD(AddressSpaces);
    DEFINE_RD(VirtualTLBPages);
    DEFINE_RD(VirtualTLBFlushEntiresSec);

    COUNTER_DATA DeviceSpacePages4K;
    COUNTER_DATA DeviceSpacePages2M;
    COUNTER_DATA DeviceSpacePages1G;
    COUNTER_DATA GPASpacePages4K;
    COUNTER_DATA GPASpacePages2M;
    COUNTER_DATA GPASpacePages1G;
    COUNTER_DATA GPASpaceModifications;
    COUNTER_DATA AttachedDevices;
    COUNTER_DATA DepositedPages;
    COUNTER_DATA DeviceDMAErrors;
    COUNTER_DATA DeviceInterruptErrors;
    COUNTER_DATA DeviceInterruptThrottleEvents;
    COUNTER_DATA IOTLBFlushesSec;
    COUNTER_DATA AddressSpaces;
    COUNTER_DATA VirtualTLBPages;
    COUNTER_DATA VirtualTLBFlushEntiresSec;
};

// Initialize the keys for the root partition metrics
void initialize_hyperv_root_partition_keys(struct hypervisor_root_partition *p)
{
    p->DeviceSpacePages4K.key = "4K device pages";
    p->DeviceSpacePages2M.key = "2M device pages";
    p->DeviceSpacePages1G.key = "1G device pages";

    p->GPASpacePages4K.key = "4K GPA pages";
    p->GPASpacePages2M.key = "2M GPA pages";
    p->GPASpacePages1G.key = "1G GPA pages";

    p->GPASpaceModifications.key = "GPA Space Modifications/sec";
    p->AttachedDevices.key = "Attached Devices";
    p->DepositedPages.key = "Deposited Pages";

    p->DeviceDMAErrors.key = "Device DMA Errors";
    p->DeviceInterruptErrors.key = "Device Interrupt Errors";
    p->DeviceInterruptThrottleEvents.key = "Device Interrupt Throttle Events";
    p->IOTLBFlushesSec.key = "I/O TLB Flushes/sec";
    p->AddressSpaces.key = "Address Spaces";
    p->VirtualTLBPages.key = "Virtual TLB Pages";
    p->VirtualTLBFlushEntiresSec.key = "Virtual TLB Flush Entries/sec";
}

// Callback function for inserting root partition metrics into the dictionary
void dict_hyperv_root_partition_insert_cb(
    const DICTIONARY_ITEM *item __maybe_unused,
    void *value,
    void *data __maybe_unused)
{
    struct hypervisor_root_partition *p = value;
    initialize_hyperv_root_partition_keys(p);
}

// Function to handle "Hyper-V Hypervisor Root Partition" metrics (Device Space and GPA Space)
static bool do_hyperv_root_partition(PERF_DATA_BLOCK *pDataBlock, int update_every, void *data)
{
    hyperv_perf_item *item = data;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, item->registry_name);
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        get_and_sanitize_instance_value(
            pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer));

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct hypervisor_root_partition *p = dictionary_set(item->instance, windows_shared_buffer, NULL, sizeof(*p));

        if (!p->collected_metadata) {
            p->collected_metadata = true;
        }

        // Fetch counters
        GET_INSTANCE_COUNTER(DeviceSpacePages4K);
        GET_INSTANCE_COUNTER(DeviceSpacePages2M);
        GET_INSTANCE_COUNTER(DeviceSpacePages1G);
        GET_INSTANCE_COUNTER(GPASpacePages4K);
        GET_INSTANCE_COUNTER(GPASpacePages2M);
        GET_INSTANCE_COUNTER(GPASpacePages1G);
        GET_INSTANCE_COUNTER(GPASpaceModifications);
        GET_INSTANCE_COUNTER(AttachedDevices);
        GET_INSTANCE_COUNTER(DepositedPages);

        GET_INSTANCE_COUNTER(DeviceDMAErrors);
        GET_INSTANCE_COUNTER(DeviceInterruptErrors);
        GET_INSTANCE_COUNTER(DeviceInterruptThrottleEvents);
        GET_INSTANCE_COUNTER(IOTLBFlushesSec);
        GET_INSTANCE_COUNTER(AddressSpaces);
        GET_INSTANCE_COUNTER(VirtualTLBPages);
        GET_INSTANCE_COUNTER(VirtualTLBFlushEntiresSec);

        // Create charts
        if (!p->charts_created) {
            p->charts_created = true;
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s", windows_shared_buffer);
            netdata_fix_chart_name(id);

            p->st_device_space_pages = rrdset_create_localhost(
                "root_partition_device_space_pages",
                id,
                NULL,
                HYPERV,
                HYPERV ".root_partition_device_space_pages",
                "Root partition device space pages",
                "pages",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_DEVICE_SPACE_PAGES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_DeviceSpacePages4K = rrddim_add(p->st_device_space_pages, "4K", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            p->rd_DeviceSpacePages2M = rrddim_add(p->st_device_space_pages, "2M", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            p->rd_DeviceSpacePages1G = rrddim_add(p->st_device_space_pages, "1G", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_gpa_space_pages = rrdset_create_localhost(
                "root_partition_gpa_space_pages",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_gpa_space_pages",
                "Root partition GPA space pages",
                "pages",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_GPA_SPACE_PAGES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_GPASpacePages4K = rrddim_add(p->st_gpa_space_pages, "4K", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            p->rd_GPASpacePages2M = rrddim_add(p->st_gpa_space_pages, "2M", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            p->rd_GPASpacePages1G = rrddim_add(p->st_gpa_space_pages, "1G", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_gpa_space_modifications = rrdset_create_localhost(
                "root_partition_gpa_space_modifications",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_gpa_space_modifications",
                "Root partition GPA space modifications",
                "modifications/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_GPA_SPACE_MODIFICATIONS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_GPASpaceModifications =
                rrddim_add(p->st_gpa_space_modifications, "gpa", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_attached_devices = rrdset_create_localhost(
                "root_partition_attached_devices",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_attached_devices",
                "Root partition attached devices",
                "devices",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_ATTACHED_DEVICES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_AttachedDevices = rrddim_add(p->st_attached_devices, "attached", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_deposited_pages = rrdset_create_localhost(
                "root_partition_deposited_pages",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_deposited_pages",
                "Root partition deposited pages",
                "pages",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_DEPOSITED_PAGES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_DepositedPages = rrddim_add(p->st_deposited_pages, "gpa", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_DeviceDMAErrors = rrdset_create_localhost(
                "root_partition_device_dma_errors",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_device_dma_errors",
                "Root partition illegal DMA requests",
                "requests",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_DEVICE_DMA_ERRORS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_DeviceDMAErrors =
                rrddim_add(p->st_DeviceDMAErrors, "illegal_dma", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_DeviceInterruptErrors = rrdset_create_localhost(
                "root_partition_device_interrupt_errors",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_device_interrupt_errors",
                "Root partition illegal interrupt requests",
                "requests",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_DEVICE_INTERRUPT_ERRORS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_DeviceInterruptErrors =
                rrddim_add(p->st_DeviceInterruptErrors, "illegal_interrupt", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_DeviceInterruptThrottleEvents = rrdset_create_localhost(
                "root_partition_device_interrupt_throttle_events",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_device_interrupt_throttle_events",
                "Root partition throttled interrupts",
                "events",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_DEVICE_INTERRUPT_THROTTLE_EVENTS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_DeviceInterruptThrottleEvents =
                rrddim_add(p->st_DeviceInterruptThrottleEvents, "throttling", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_IOTLBFlushesSec = rrdset_create_localhost(
                "root_partition_io_tlb_flush",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_io_tlb_flush",
                "Root partition flushes of I/O TLBs",
                "flushes/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_IO_TLB_FLUSH,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_IOTLBFlushesSec = rrddim_add(p->st_IOTLBFlushesSec, "gpa", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            p->st_AddressSpaces = rrdset_create_localhost(
                "root_partition_address_space",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_address_space",
                "Root partition address spaces in the virtual TLB",
                "address spaces",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_ADDRESS_SPACE,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_AddressSpaces = rrddim_add(p->st_AddressSpaces, "address_spaces", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_VirtualTLBPages = rrdset_create_localhost(
                "root_partition_virtual_tlb_pages",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_virtual_tlb_pages",
                "Root partition pages used by the virtual TLB",
                "pages",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_VIRTUAL_TLB_PAGES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_VirtualTLBPages = rrddim_add(p->st_VirtualTLBPages, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->st_VirtualTLBFlushEntiresSec = rrdset_create_localhost(
                "root_partition_virtual_tlb_flush_entries",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".root_partition_virtual_tlb_flush_entries",
                "Root partition flushes of the entire virtual TLB",
                "flushes/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_ROOT_PARTITION_VIRTUAL_TLB_FLUSH_ENTRIES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_VirtualTLBFlushEntiresSec =
                rrddim_add(p->st_VirtualTLBFlushEntiresSec, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        // Set the data for each dimension

        SETP_DIM_VALUE(st_device_space_pages, DeviceSpacePages4K);
        SETP_DIM_VALUE(st_device_space_pages, DeviceSpacePages2M);
        SETP_DIM_VALUE(st_device_space_pages, DeviceSpacePages1G);

        SETP_DIM_VALUE(st_gpa_space_pages, GPASpacePages4K);
        SETP_DIM_VALUE(st_gpa_space_pages, GPASpacePages2M);
        SETP_DIM_VALUE(st_gpa_space_pages, GPASpacePages1G);

        SETP_DIM_VALUE(st_gpa_space_modifications, GPASpaceModifications);

        SETP_DIM_VALUE(st_attached_devices, AttachedDevices);
        SETP_DIM_VALUE(st_deposited_pages, DepositedPages);

        SETP_DIM_VALUE(st_DeviceDMAErrors, DeviceDMAErrors);
        SETP_DIM_VALUE(st_DeviceInterruptErrors, DeviceInterruptErrors);
        SETP_DIM_VALUE(st_DeviceInterruptThrottleEvents, DeviceInterruptThrottleEvents);
        SETP_DIM_VALUE(st_IOTLBFlushesSec, IOTLBFlushesSec);
        SETP_DIM_VALUE(st_AddressSpaces, AddressSpaces);
        SETP_DIM_VALUE(st_VirtualTLBPages, VirtualTLBPages);
        SETP_DIM_VALUE(st_VirtualTLBFlushEntiresSec, VirtualTLBFlushEntiresSec);

        // Mark the charts as done
        rrdset_done(p->st_device_space_pages);
        rrdset_done(p->st_gpa_space_pages);
        rrdset_done(p->st_gpa_space_modifications);
        rrdset_done(p->st_attached_devices);
        rrdset_done(p->st_deposited_pages);
        rrdset_done(p->st_DeviceInterruptErrors);
        rrdset_done(p->st_DeviceInterruptThrottleEvents);
        rrdset_done(p->st_IOTLBFlushesSec);
        rrdset_done(p->st_AddressSpaces);
        rrdset_done(p->st_DeviceDMAErrors);
        rrdset_done(p->st_VirtualTLBPages);
        rrdset_done(p->st_VirtualTLBFlushEntiresSec);
    }

    return true;
}

// Storage DEVICE

struct hypervisor_storage_device {
    bool collected_metadata;
    bool charts_created;

    RRDSET *st_operations;
    DEFINE_RD(ReadOperationsSec);
    DEFINE_RD(WriteOperationsSec);

    RRDSET *st_bytes;
    DEFINE_RD(ReadBytesSec);
    DEFINE_RD(WriteBytesSec);

    RRDSET *st_errors;
    DEFINE_RD(ErrorCount);

    COUNTER_DATA ReadOperationsSec;
    COUNTER_DATA WriteOperationsSec;

    COUNTER_DATA ReadBytesSec;
    COUNTER_DATA WriteBytesSec;
    COUNTER_DATA ErrorCount;
};

// Initialize the keys for the root partition metrics
void initialize_hyperv_storage_device_keys(struct hypervisor_storage_device *p)
{
    p->ReadOperationsSec.key = "Read Operations/Sec";
    p->WriteOperationsSec.key = "Write Operations/Sec";

    p->ReadBytesSec.key = "Read Bytes/sec";
    p->WriteBytesSec.key = "Write Bytes/sec";
    p->ErrorCount.key = "Error Count";
}

// Callback function for inserting root partition metrics into the dictionary
void dict_hyperv_storage_device_insert_cb(
    const DICTIONARY_ITEM *item __maybe_unused,
    void *value,
    void *data __maybe_unused)
{
    struct hypervisor_storage_device *p = value;
    initialize_hyperv_storage_device_keys(p);
}

static bool do_hyperv_storage_device(PERF_DATA_BLOCK *pDataBlock, int update_every, void *data)
{
    hyperv_perf_item *item = data;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, item->registry_name);
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        get_and_sanitize_instance_value(
            pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer));

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct hypervisor_storage_device *p = dictionary_set(item->instance, windows_shared_buffer, NULL, sizeof(*p));

        if (!p->collected_metadata) {
            p->collected_metadata = true;
        }

        // Fetch counters
        GET_INSTANCE_COUNTER(ReadOperationsSec);
        GET_INSTANCE_COUNTER(WriteOperationsSec);

        GET_INSTANCE_COUNTER(ReadBytesSec);
        GET_INSTANCE_COUNTER(WriteBytesSec);
        GET_INSTANCE_COUNTER(ErrorCount);

        if (!p->charts_created) {
            p->charts_created = true;
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s", windows_shared_buffer);
            netdata_fix_chart_name(id);

            if (!p->st_operations) {
                p->st_operations = rrdset_create_localhost(
                    "vm_storage_device_operations",
                    id,
                    NULL,
                    HYPERV,
                    HYPERV ".vm_storage_device_operations",
                    "VM storage device IOPS",
                    "operations/s",
                    _COMMON_PLUGIN_NAME,
                    _COMMON_PLUGIN_MODULE_NAME,
                    NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_STORAGE_DEVICE_OPERATIONS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_ReadOperationsSec = rrddim_add(p->st_operations, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                p->rd_WriteOperationsSec =
                    rrddim_add(p->st_operations, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_operations->rrdlabels, "vm_storage_device", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            if (!p->st_bytes) {
                p->st_bytes = rrdset_create_localhost(
                    "vm_storage_device_bytes",
                    windows_shared_buffer,
                    NULL,
                    HYPERV,
                    HYPERV ".vm_storage_device_bytes",
                    "VM storage device IO",
                    "bytes/s",
                    _COMMON_PLUGIN_NAME,
                    _COMMON_PLUGIN_MODULE_NAME,
                    NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_STORAGE_DEVICE_BYTES,
                    update_every,
                    RRDSET_TYPE_AREA);

                p->rd_ReadBytesSec = rrddim_add(p->st_bytes, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                p->rd_WriteBytesSec = rrddim_add(p->st_bytes, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_bytes->rrdlabels, "vm_storage_device", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            if (!p->st_errors) {
                p->st_errors = rrdset_create_localhost(
                    "vm_storage_device_errors",
                    windows_shared_buffer,
                    NULL,
                    HYPERV,
                    HYPERV ".vm_storage_device_errors",
                    "VM storage device errors",
                    "errors/s",
                    _COMMON_PLUGIN_NAME,
                    _COMMON_PLUGIN_MODULE_NAME,
                    NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_STORAGE_DEVICE_ERRORS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_ErrorCount = rrddim_add(p->st_errors, "errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_errors->rrdlabels, "vm_storage_device", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }
        }

        SETP_DIM_VALUE(st_operations, ReadOperationsSec);
        SETP_DIM_VALUE(st_operations, WriteOperationsSec);

        SETP_DIM_VALUE(st_bytes, ReadBytesSec);
        SETP_DIM_VALUE(st_bytes, WriteBytesSec);

        SETP_DIM_VALUE(st_errors, ErrorCount);

        // Mark the charts as done
        rrdset_done(p->st_operations);
        rrdset_done(p->st_bytes);
        rrdset_done(p->st_errors);
    }

    return true;
}

struct hypervisor_switch {
    bool collected_metadata;
    bool charts_created;

    RRDSET *st_bytes;
    DEFINE_RD(BytesSentSec);
    DEFINE_RD(BytesReceivedSec);

    RRDSET *st_packets;
    DEFINE_RD(PacketsSentSec);
    DEFINE_RD(PacketsReceivedSec);

    RRDSET *st_directed_packets;
    DEFINE_RD(DirectedPacketsSentSec);
    DEFINE_RD(DirectedPacketsReceivedSec);

    RRDSET *st_broadcast_packets;
    DEFINE_RD(BroadcastPacketsSentSec);
    DEFINE_RD(BroadcastPacketsReceivedSec);

    RRDSET *st_multicast_packets;
    DEFINE_RD(MulticastPacketsSentSec);
    DEFINE_RD(MulticastPacketsReceivedSec);

    RRDSET *st_dropped_packets;
    DEFINE_RD(DroppedPacketsOutgoingSec);
    DEFINE_RD(DroppedPacketsIncomingSec);

    RRDSET *st_ext_dropped_packets;
    DEFINE_RD(ExtensionsDroppedPacketsOutgoingSec);
    DEFINE_RD(ExtensionsDroppedPacketsIncomingSec);

    RRDSET *st_flooded;
    DEFINE_RD(PacketsFlooded);

    RRDSET *st_learned_mac;
    DEFINE_RD(LearnedMacAddresses);

    RRDSET *st_purged_mac;
    DEFINE_RD(PurgedMacAddresses);

    COUNTER_DATA BytesSentSec;
    COUNTER_DATA BytesReceivedSec;

    COUNTER_DATA PacketsSentSec;
    COUNTER_DATA PacketsReceivedSec;

    COUNTER_DATA DirectedPacketsSentSec;
    COUNTER_DATA DirectedPacketsReceivedSec;

    COUNTER_DATA BroadcastPacketsSentSec;
    COUNTER_DATA BroadcastPacketsReceivedSec;

    COUNTER_DATA MulticastPacketsSentSec;
    COUNTER_DATA MulticastPacketsReceivedSec;

    COUNTER_DATA DroppedPacketsOutgoingSec;
    COUNTER_DATA DroppedPacketsIncomingSec;

    COUNTER_DATA ExtensionsDroppedPacketsOutgoingSec;
    COUNTER_DATA ExtensionsDroppedPacketsIncomingSec;

    COUNTER_DATA PacketsFlooded;

    COUNTER_DATA LearnedMacAddresses;

    COUNTER_DATA PurgedMacAddresses;
};

// Initialize the keys for the root partition metrics
void initialize_hyperv_switch_keys(struct hypervisor_switch *p)
{
    p->BytesSentSec.key = "Bytes Sent/sec";
    p->BytesReceivedSec.key = "Bytes Received/sec";
    p->PacketsSentSec.key = "Packets Sent/sec";
    p->PacketsReceivedSec.key = "Packets Received/sec";

    p->DirectedPacketsSentSec.key = "Directed Packets Sent/sec";
    p->DirectedPacketsReceivedSec.key = "Directed Packets Received/sec";
    p->BroadcastPacketsSentSec.key = "Broadcast Packets Sent/sec";
    p->BroadcastPacketsReceivedSec.key = "Broadcast Packets Received/sec";
    p->MulticastPacketsSentSec.key = "Multicast Packets Sent/sec";
    p->MulticastPacketsReceivedSec.key = "Multicast Packets Received/sec";
    p->DroppedPacketsOutgoingSec.key = "Dropped Packets Outgoing/sec";
    p->DroppedPacketsIncomingSec.key = "Dropped Packets Incoming/sec";
    p->ExtensionsDroppedPacketsOutgoingSec.key = "Extensions Dropped Packets Outgoing/sec";
    p->ExtensionsDroppedPacketsIncomingSec.key = "Extensions Dropped Packets Incoming/sec";
    p->PacketsFlooded.key = "Packets Flooded";
    p->LearnedMacAddresses.key = "Learned Mac Addresses";
    p->PurgedMacAddresses.key = "Purged Mac Addresses";
}

void dict_hyperv_switch_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct hypervisor_switch *p = value;
    initialize_hyperv_switch_keys(p);
}

static bool do_hyperv_switch(PERF_DATA_BLOCK *pDataBlock, int update_every, void *data)
{
    hyperv_perf_item *item = data;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, item->registry_name);
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        get_and_sanitize_instance_value(
            pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer));

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct hypervisor_switch *p = dictionary_set(item->instance, windows_shared_buffer, NULL, sizeof(*p));

        if (!p->collected_metadata) {
            p->collected_metadata = true;
        }

        GET_INSTANCE_COUNTER(BytesReceivedSec);
        GET_INSTANCE_COUNTER(BytesSentSec);

        GET_INSTANCE_COUNTER(PacketsReceivedSec);
        GET_INSTANCE_COUNTER(PacketsSentSec);

        GET_INSTANCE_COUNTER(DirectedPacketsSentSec);
        GET_INSTANCE_COUNTER(DirectedPacketsReceivedSec);

        GET_INSTANCE_COUNTER(BroadcastPacketsSentSec);
        GET_INSTANCE_COUNTER(BroadcastPacketsReceivedSec);

        GET_INSTANCE_COUNTER(MulticastPacketsSentSec);
        GET_INSTANCE_COUNTER(MulticastPacketsReceivedSec);

        GET_INSTANCE_COUNTER(DroppedPacketsOutgoingSec);
        GET_INSTANCE_COUNTER(DroppedPacketsIncomingSec);

        GET_INSTANCE_COUNTER(ExtensionsDroppedPacketsOutgoingSec);
        GET_INSTANCE_COUNTER(ExtensionsDroppedPacketsIncomingSec);

        GET_INSTANCE_COUNTER(PacketsFlooded);

        GET_INSTANCE_COUNTER(LearnedMacAddresses);

        GET_INSTANCE_COUNTER(PurgedMacAddresses);

        if (!p->charts_created) {
            p->charts_created = true;
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s", windows_shared_buffer);
            netdata_fix_chart_name(id);

            p->st_bytes = rrdset_create_localhost(
                "vswitch_traffic",
                id,
                NULL,
                HYPERV,
                HYPERV ".vswitch_traffic",
                "Virtual switch traffic",
                "kilobits/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_TRAFFIC,
                update_every,
                RRDSET_TYPE_AREA);

            p->rd_BytesReceivedSec = rrddim_add(p->st_bytes, "received", NULL, 8, 1000, RRD_ALGORITHM_INCREMENTAL);
            p->rd_BytesSentSec = rrddim_add(p->st_bytes, "sent", NULL, -8, 1000, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_bytes->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_packets = rrdset_create_localhost(
                "vswitch_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vswitch_packets",
                "Virtual switch packets",
                "packets/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_PacketsReceivedSec = rrddim_add(p->st_packets, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_PacketsSentSec = rrddim_add(p->st_packets, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_packets->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_directed_packets = rrdset_create_localhost(
                "vswitch_directed_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vswitch_directed_packets",
                "Virtual switch directed packets",
                "packets/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_DIRECTED_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_DirectedPacketsReceivedSec =
                rrddim_add(p->st_directed_packets, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_DirectedPacketsSentSec =
                rrddim_add(p->st_directed_packets, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_directed_packets->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_broadcast_packets = rrdset_create_localhost(
                "vswitch_broadcast_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vswitch_broadcast_packets",
                "Virtual switch broadcast packets",
                "packets/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_BROADCAST_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_BroadcastPacketsReceivedSec =
                rrddim_add(p->st_broadcast_packets, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_BroadcastPacketsSentSec =
                rrddim_add(p->st_broadcast_packets, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_broadcast_packets->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_multicast_packets = rrdset_create_localhost(
                "vswitch_multicast_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vswitch_multicast_packets",
                "Virtual switch multicast packets",
                "packets/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_MULTICAST_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_MulticastPacketsReceivedSec =
                rrddim_add(p->st_multicast_packets, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_MulticastPacketsSentSec =
                rrddim_add(p->st_multicast_packets, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_multicast_packets->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_dropped_packets = rrdset_create_localhost(
                "vswitch_dropped_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vswitch_dropped_packets",
                "Virtual switch dropped packets",
                "drops/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_DROPPED_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_DroppedPacketsIncomingSec =
                rrddim_add(p->st_dropped_packets, "incoming", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_DroppedPacketsOutgoingSec =
                rrddim_add(p->st_dropped_packets, "outgoing", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_dropped_packets->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_ext_dropped_packets = rrdset_create_localhost(
                "vswitch_extensions_dropped_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vswitch_extensions_dropped_packets",
                "Virtual switch extensions dropped packets",
                "drops/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_EXTENSIONS_DROPPED_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_ExtensionsDroppedPacketsIncomingSec =
                rrddim_add(p->st_ext_dropped_packets, "incoming", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_ExtensionsDroppedPacketsOutgoingSec =
                rrddim_add(p->st_ext_dropped_packets, "outgoing", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_ext_dropped_packets->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_flooded = rrdset_create_localhost(
                "vswitch_packets_flooded",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vswitch_packets_flooded",
                "Virtual switch flooded packets",
                "packets/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_PACKETS_FLOODED,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_PacketsFlooded = rrddim_add(p->st_flooded, "flooded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_flooded->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_learned_mac = rrdset_create_localhost(
                "vswitch_learned_mac_addresses",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vswitch_learned_mac_addresses",
                "Virtual switch learned MAC addresses",
                "mac addresses/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_LEARNED_MAC_ADDRESSES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_LearnedMacAddresses = rrddim_add(p->st_learned_mac, "learned", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_learned_mac->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_purged_mac = rrdset_create_localhost(
                "vswitch_purged_mac_addresses",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vswitch_purged_mac_addresses",
                "Virtual switch purged MAC addresses",
                "mac addresses/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VSWITCH_PURGED_MAC_ADDRESSES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_PurgedMacAddresses = rrddim_add(p->st_purged_mac, "purged", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_purged_mac->rrdlabels, "vswitch", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        SETP_DIM_VALUE(st_packets, PacketsReceivedSec);
        SETP_DIM_VALUE(st_packets, PacketsSentSec);

        SETP_DIM_VALUE(st_bytes, BytesReceivedSec);
        SETP_DIM_VALUE(st_bytes, BytesSentSec);

        SETP_DIM_VALUE(st_directed_packets, DirectedPacketsSentSec);
        SETP_DIM_VALUE(st_directed_packets, DirectedPacketsReceivedSec);

        SETP_DIM_VALUE(st_broadcast_packets, BroadcastPacketsSentSec);
        SETP_DIM_VALUE(st_broadcast_packets, BroadcastPacketsReceivedSec);

        SETP_DIM_VALUE(st_multicast_packets, MulticastPacketsSentSec);
        SETP_DIM_VALUE(st_multicast_packets, MulticastPacketsReceivedSec);

        SETP_DIM_VALUE(st_dropped_packets, DroppedPacketsOutgoingSec);
        SETP_DIM_VALUE(st_dropped_packets, DroppedPacketsIncomingSec);

        SETP_DIM_VALUE(st_ext_dropped_packets, ExtensionsDroppedPacketsOutgoingSec);
        SETP_DIM_VALUE(st_ext_dropped_packets, ExtensionsDroppedPacketsIncomingSec);

        SETP_DIM_VALUE(st_flooded, PacketsFlooded);
        SETP_DIM_VALUE(st_learned_mac, LearnedMacAddresses);
        SETP_DIM_VALUE(st_purged_mac, PurgedMacAddresses);

        // Mark the charts as done
        rrdset_done(p->st_packets);
        rrdset_done(p->st_bytes);

        rrdset_done(p->st_directed_packets);
        rrdset_done(p->st_broadcast_packets);
        rrdset_done(p->st_multicast_packets);
        rrdset_done(p->st_dropped_packets);
        rrdset_done(p->st_ext_dropped_packets);
        rrdset_done(p->st_flooded);
        rrdset_done(p->st_learned_mac);
        rrdset_done(p->st_purged_mac);
    }
    return true;
}

struct hypervisor_network_adapter {
    bool collected_metadata;
    bool charts_created;

    RRDSET *st_dropped_packets;
    DEFINE_RD(DroppedPacketsOutgoingSec);
    DEFINE_RD(DroppedPacketsIncomingSec);

    RRDSET *st_send_receive_packets;
    DEFINE_RD(PacketsSentSec);
    DEFINE_RD(PacketsReceivedSec);

    RRDSET *st_send_receive_bytes;
    DEFINE_RD(BytesSentSec);
    DEFINE_RD(BytesReceivedSec);

    RRDSET *st_IPsecoffloadBytes;
    DEFINE_RD(IPsecoffloadBytesReceivedSec);
    DEFINE_RD(IPsecoffloadBytesSentSec);

    RRDSET *st_DirectedPackets;
    DEFINE_RD(DirectedPacketsSentSec);
    DEFINE_RD(DirectedPacketsReceivedSec);

    RRDSET *st_BroadcastPackets;
    DEFINE_RD(BroadcastPacketsSentSec);
    DEFINE_RD(BroadcastPacketsReceivedSec);

    RRDSET *st_MulticastPackets;
    DEFINE_RD(MulticastPacketsSentSec);
    DEFINE_RD(MulticastPacketsReceivedSec);

    COUNTER_DATA DroppedPacketsOutgoingSec;
    COUNTER_DATA DroppedPacketsIncomingSec;

    COUNTER_DATA PacketsSentSec;
    COUNTER_DATA PacketsReceivedSec;

    COUNTER_DATA BytesSentSec;
    COUNTER_DATA BytesReceivedSec;

    COUNTER_DATA IPsecoffloadBytesReceivedSec;
    COUNTER_DATA IPsecoffloadBytesSentSec;

    COUNTER_DATA DirectedPacketsSentSec;
    COUNTER_DATA DirectedPacketsReceivedSec;

    COUNTER_DATA BroadcastPacketsSentSec;
    COUNTER_DATA BroadcastPacketsReceivedSec;

    COUNTER_DATA MulticastPacketsSentSec;
    COUNTER_DATA MulticastPacketsReceivedSec;
};

// Initialize the keys for the root partition metrics
void initialize_hyperv_network_adapter_keys(struct hypervisor_network_adapter *p)
{
    p->DroppedPacketsOutgoingSec.key = "Dropped Packets Outgoing/sec";
    p->DroppedPacketsIncomingSec.key = "Dropped Packets Incoming/sec";

    p->PacketsSentSec.key = "Packets Sent/sec";
    p->PacketsReceivedSec.key = "Packets Received/sec";

    p->BytesSentSec.key = "Bytes Sent/sec";
    p->BytesReceivedSec.key = "Bytes Received/sec";

    p->IPsecoffloadBytesReceivedSec.key = "IPsec offload Bytes Receive/sec";
    p->IPsecoffloadBytesSentSec.key = "IPsec offload Bytes Sent/sec";
    p->DirectedPacketsSentSec.key = "Directed Packets Sent/sec";
    p->DirectedPacketsReceivedSec.key = "Directed Packets Received/sec";
    p->BroadcastPacketsSentSec.key = "Broadcast Packets Sent/sec";
    p->BroadcastPacketsReceivedSec.key = "Broadcast Packets Received/sec";
    p->MulticastPacketsSentSec.key = "Multicast Packets Sent/sec";
    p->MulticastPacketsReceivedSec.key = "Multicast Packets Received/sec";
}

void dict_hyperv_network_adapter_insert_cb(
    const DICTIONARY_ITEM *item __maybe_unused,
    void *value,
    void *data __maybe_unused)
{
    struct hypervisor_network_adapter *p = value;
    initialize_hyperv_network_adapter_keys(p);
}

static bool do_hyperv_network_adapter(PERF_DATA_BLOCK *pDataBlock, int update_every, void *data)
{
    hyperv_perf_item *item = data;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, item->registry_name);
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        get_and_sanitize_instance_value(
            pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer));

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct hypervisor_network_adapter *p = dictionary_set(item->instance, windows_shared_buffer, NULL, sizeof(*p));

        if (!p->collected_metadata) {
            p->collected_metadata = true;
        }

        GET_INSTANCE_COUNTER(DroppedPacketsIncomingSec);
        GET_INSTANCE_COUNTER(DroppedPacketsOutgoingSec);

        GET_INSTANCE_COUNTER(PacketsReceivedSec);
        GET_INSTANCE_COUNTER(PacketsSentSec);

        GET_INSTANCE_COUNTER(BytesReceivedSec);
        GET_INSTANCE_COUNTER(BytesSentSec);

        GET_INSTANCE_COUNTER(IPsecoffloadBytesReceivedSec);
        GET_INSTANCE_COUNTER(IPsecoffloadBytesSentSec);

        GET_INSTANCE_COUNTER(DirectedPacketsSentSec);
        GET_INSTANCE_COUNTER(DirectedPacketsReceivedSec);

        GET_INSTANCE_COUNTER(BroadcastPacketsSentSec);
        GET_INSTANCE_COUNTER(BroadcastPacketsReceivedSec);

        GET_INSTANCE_COUNTER(MulticastPacketsSentSec);
        GET_INSTANCE_COUNTER(MulticastPacketsReceivedSec);

        if (!p->charts_created) {
            p->charts_created = true;
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s", windows_shared_buffer);
            netdata_fix_chart_name(id);

            p->st_dropped_packets = rrdset_create_localhost(
                "vm_net_interface_packets_dropped",
                id,
                NULL,
                HYPERV,
                HYPERV ".vm_net_interface_packets_dropped",
                "VM interface packets dropped",
                "drops/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_NET_INTERFACE_PACKETS_DROPPED,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_DroppedPacketsIncomingSec =
                rrddim_add(p->st_dropped_packets, "incoming", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_DroppedPacketsOutgoingSec =
                rrddim_add(p->st_dropped_packets, "outgoing", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(
                p->st_dropped_packets->rrdlabels, "vm_net_interface", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_send_receive_packets = rrdset_create_localhost(
                "vm_net_interface_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vm_net_interface_packets",
                "VM interface packets",
                "packets/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_NET_INTERFACE_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_PacketsReceivedSec =
                rrddim_add(p->st_send_receive_packets, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_PacketsSentSec =
                rrddim_add(p->st_send_receive_packets, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(
                p->st_send_receive_packets->rrdlabels, "vm_net_interface", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_send_receive_bytes = rrdset_create_localhost(
                "vm_net_interface_traffic",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vm_net_interface_traffic",
                "VM interface traffic",
                "kilobits/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_NET_INTERFACE_TRAFFIC,
                update_every,
                RRDSET_TYPE_AREA);

            p->rd_BytesReceivedSec =
                rrddim_add(p->st_send_receive_bytes, "received", NULL, 8, 1000, RRD_ALGORITHM_INCREMENTAL);
            p->rd_BytesSentSec =
                rrddim_add(p->st_send_receive_bytes, "sent", NULL, -8, 1000, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_send_receive_bytes->rrdlabels, "vm_net_interface", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_IPsecoffloadBytes = rrdset_create_localhost(
                "vm_net_interface_ipsec_traffic",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vm_net_interface_ipsec_traffic",
                "VM interface IPSec traffic",
                "kilobits/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_NET_INTERFACE_IPSEC_TRAFFIC,
                update_every,
                RRDSET_TYPE_AREA);

            p->rd_IPsecoffloadBytesReceivedSec =
                rrddim_add(p->st_IPsecoffloadBytes, "received", NULL, 8, 1000, RRD_ALGORITHM_INCREMENTAL);
            p->rd_IPsecoffloadBytesSentSec =
                rrddim_add(p->st_IPsecoffloadBytes, "sent", NULL, -8, 1000, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_IPsecoffloadBytes->rrdlabels, "vm_net_interface", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_DirectedPackets = rrdset_create_localhost(
                "vm_net_interface_directed_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vm_net_interface_directed_packets",
                "VM interface directed packets",
                "packets/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_NET_INTERFACE_DIRECTED_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_DirectedPacketsReceivedSec =
                rrddim_add(p->st_DirectedPackets, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_DirectedPacketsSentSec =
                rrddim_add(p->st_DirectedPackets, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_DirectedPackets->rrdlabels, "vm_net_interface", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_BroadcastPackets = rrdset_create_localhost(
                "vm_net_interface_broadcast_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vm_net_interface_broadcast_packets",
                "VM interface broadcast",
                "packets/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_NET_INTERFACE_BROADCAST_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_BroadcastPacketsReceivedSec =
                rrddim_add(p->st_BroadcastPackets, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_BroadcastPacketsSentSec =
                rrddim_add(p->st_BroadcastPackets, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_BroadcastPackets->rrdlabels, "vm_net_interface", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_MulticastPackets = rrdset_create_localhost(
                "vm_net_interface_multicast_packets",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vm_net_interface_multicast_packets",
                "VM interface multicast",
                "packets/s",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_NET_INTERFACE_MULTICAST_PACKETS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_MulticastPacketsReceivedSec =
                rrddim_add(p->st_MulticastPackets, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_MulticastPacketsSentSec =
                rrddim_add(p->st_MulticastPackets, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_MulticastPackets->rrdlabels, "vm_net_interface", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        SETP_DIM_VALUE(st_dropped_packets, DroppedPacketsIncomingSec);
        SETP_DIM_VALUE(st_dropped_packets, DroppedPacketsOutgoingSec);

        SETP_DIM_VALUE(st_send_receive_packets, PacketsReceivedSec);
        SETP_DIM_VALUE(st_send_receive_packets, PacketsSentSec);

        SETP_DIM_VALUE(st_send_receive_bytes, BytesReceivedSec);
        SETP_DIM_VALUE(st_send_receive_bytes, BytesSentSec);

        SETP_DIM_VALUE(st_IPsecoffloadBytes, IPsecoffloadBytesReceivedSec);
        SETP_DIM_VALUE(st_IPsecoffloadBytes, IPsecoffloadBytesSentSec);

        SETP_DIM_VALUE(st_DirectedPackets, DirectedPacketsSentSec);
        SETP_DIM_VALUE(st_DirectedPackets, DirectedPacketsReceivedSec);

        SETP_DIM_VALUE(st_BroadcastPackets, BroadcastPacketsSentSec);
        SETP_DIM_VALUE(st_BroadcastPackets, BroadcastPacketsReceivedSec);

        SETP_DIM_VALUE(st_MulticastPackets, MulticastPacketsSentSec);
        SETP_DIM_VALUE(st_MulticastPackets, MulticastPacketsReceivedSec);

        rrdset_done(p->st_IPsecoffloadBytes);
        rrdset_done(p->st_DirectedPackets);
        rrdset_done(p->st_BroadcastPackets);
        rrdset_done(p->st_MulticastPackets);
        rrdset_done(p->st_send_receive_bytes);
        rrdset_done(p->st_send_receive_packets);
        rrdset_done(p->st_dropped_packets);
    }
    return true;
}

// Hypervisor Virtual Processor
struct hypervisor_processor {
    bool collected_metadata;
    bool charts_created;

    RRDSET *st_HypervisorProcessor;

    DEFINE_RD(GuestRunTime);
    DEFINE_RD(HypervisorRunTime);
    DEFINE_RD(RemoteRunTime);

    RRDSET *st_HypervisorProcessorTotal;
    DEFINE_RD(TotalRunTime);

    COUNTER_DATA GuestRunTime;
    COUNTER_DATA HypervisorRunTime;
    COUNTER_DATA RemoteRunTime;
    COUNTER_DATA TotalRunTime;
    collected_number GuestRunTime_total;
    collected_number HypervisorRunTime_total;
    collected_number RemoteRunTime_total;
    collected_number TotalRunTime_total;
};

void initialize_hyperv_processor_keys(struct hypervisor_processor *p)
{
    p->GuestRunTime.key = "% Guest Run Time";
    p->HypervisorRunTime.key = "% Hypervisor Run Time";
    p->RemoteRunTime.key = "% Remote Run Time";
    p->TotalRunTime.key = "% Total Run Time";
    p->GuestRunTime_total = 0;
    p->HypervisorRunTime_total = 0;
    p->RemoteRunTime_total = 0;
    p->TotalRunTime_total = 0;
}

void dict_hyperv_processor_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct hypervisor_processor *p = value;
    initialize_hyperv_processor_keys(p);
}

static bool do_hyperv_processor(PERF_DATA_BLOCK *pDataBlock, int update_every, void *data)
{
    hyperv_perf_item *item = data;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, item->registry_name);
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        get_and_sanitize_instance_value(
            pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer));

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        char *vm = strchr(windows_shared_buffer, ':');
        if (vm)
            *vm = '\0';

        struct hypervisor_processor *p = dictionary_set(item->instance, windows_shared_buffer, NULL, sizeof(*p));

        if (!p->collected_metadata) {
            p->collected_metadata = true;
        }

        GET_INSTANCE_COUNTER(GuestRunTime);
        GET_INSTANCE_COUNTER(HypervisorRunTime);
        GET_INSTANCE_COUNTER(RemoteRunTime);
        GET_INSTANCE_COUNTER(TotalRunTime);

        if (!p->charts_created) {
            p->charts_created = true;
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s", windows_shared_buffer);
            netdata_fix_chart_name(id);

            p->st_HypervisorProcessorTotal = rrdset_create_localhost(
                "vm_cpu_usage",
                id,
                NULL,
                HYPERV,
                HYPERV ".vm_cpu_usage",
                "VM CPU usage",
                "percentage",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_CPU_USAGE,
                update_every,
                RRDSET_TYPE_AREA);

            p->rd_TotalRunTime =
                rrddim_add(p->st_HypervisorProcessorTotal, "usage", NULL, 1, 1000000, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_HypervisorProcessorTotal->rrdlabels, "vm_name", windows_shared_buffer, RRDLABEL_SRC_AUTO);

            p->st_HypervisorProcessor = rrdset_create_localhost(
                "vm_cpu_usage_by_run_context",
                windows_shared_buffer,
                NULL,
                HYPERV,
                HYPERV ".vm_cpu_usage_by_run_context",
                "VM CPU usage by run context",
                "percentage",
                _COMMON_PLUGIN_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_WINDOWS_HYPERV_VM_CPU_USAGE_BY_RUN_CONTEXT,
                update_every,
                RRDSET_TYPE_STACKED);

            p->rd_GuestRunTime =
                rrddim_add(p->st_HypervisorProcessor, "guest", NULL, 1, 1000000, RRD_ALGORITHM_INCREMENTAL);
            p->rd_HypervisorRunTime =
                rrddim_add(p->st_HypervisorProcessor, "hypervisor", NULL, 1, 1000000, RRD_ALGORITHM_INCREMENTAL);
            p->rd_RemoteRunTime =
                rrddim_add(p->st_HypervisorProcessor, "remote", NULL, 1, 1000000, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_HypervisorProcessor->rrdlabels, "vm_name", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        p->GuestRunTime_total += (collected_number)p->GuestRunTime.current.Data;
        p->HypervisorRunTime_total += (collected_number)p->HypervisorRunTime.current.Data;
        p->RemoteRunTime_total += (collected_number)p->RemoteRunTime.current.Data;
        p->TotalRunTime_total += (collected_number)p->TotalRunTime.current.Data;
    }

    {
        struct hypervisor_processor *p;
        dfe_start_read(item->instance, p)
        {
            rrddim_set_by_pointer(
                p->st_HypervisorProcessor, p->rd_HypervisorRunTime, (collected_number)p->HypervisorRunTime_total);
            rrddim_set_by_pointer(
                p->st_HypervisorProcessor, p->rd_GuestRunTime, (collected_number)p->GuestRunTime_total);
            rrddim_set_by_pointer(
                p->st_HypervisorProcessor, p->rd_RemoteRunTime, (collected_number)p->RemoteRunTime_total);
            rrdset_done(p->st_HypervisorProcessor);

            rrddim_set_by_pointer(
                p->st_HypervisorProcessorTotal, p->rd_TotalRunTime, (collected_number)p->TotalRunTime_total);
            rrdset_done(p->st_HypervisorProcessorTotal);

            p->GuestRunTime_total = 0;
            p->HypervisorRunTime_total = 0;
            p->RemoteRunTime_total = 0;
            p->TotalRunTime_total = 0;
        }
        dfe_done(p);
    }

    return true;
}

hyperv_perf_item hyperv_perf_list[] = {
    {.registry_name = "Hyper-V Dynamic Memory VM",
     .function_collect = do_hyperv_memory,
     .dict_insert_cb = dict_hyperv_memory_insert_cb,
     .dict_size = sizeof(struct hypervisor_memory)},

    {.registry_name = "Hyper-V VM Vid Partition",
     .function_collect = do_hyperv_vid_partition,
     .dict_insert_cb = dict_hyperv_partition_insert_cb,
     .dict_size = sizeof(struct hypervisor_partition)},

    {
        .registry_name = "Hyper-V Virtual Machine Health Summary",
        .function_collect = do_hyperv_health_summary,
    },

    {
        .registry_name = "Hyper-V Hypervisor Root Partition",
        .function_collect = do_hyperv_root_partition,
        .dict_insert_cb = dict_hyperv_root_partition_insert_cb,
        .dict_size = sizeof(struct hypervisor_root_partition),
    },

    {.registry_name = "Hyper-V Virtual Storage Device",
     .function_collect = do_hyperv_storage_device,
     .dict_insert_cb = dict_hyperv_storage_device_insert_cb,
     .dict_size = sizeof(struct hypervisor_storage_device)},

    {.registry_name = "Hyper-V Virtual Switch",
     .function_collect = do_hyperv_switch,
     .dict_insert_cb = dict_hyperv_switch_insert_cb,
     .dict_size = sizeof(struct hypervisor_switch)},

    {.registry_name = "Hyper-V Virtual Network Adapter",
     .function_collect = do_hyperv_network_adapter,
     .dict_insert_cb = dict_hyperv_network_adapter_insert_cb,
     .dict_size = sizeof(struct hypervisor_network_adapter)},

    {.registry_name = "Hyper-V Hypervisor Virtual Processor",
     .function_collect = do_hyperv_processor,
     .dict_insert_cb = dict_hyperv_processor_insert_cb,
     .dict_size = sizeof(struct hypervisor_processor)},

    {.registry_name = NULL, .function_collect = NULL}};

int do_PerflibHyperV(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        for (int i = 0; hyperv_perf_list[i].registry_name != NULL; i++) {
            hyperv_perf_item *item = &hyperv_perf_list[i];
            if (item->dict_insert_cb) {
                item->instance = dictionary_create_advanced(DICT_PERF_OPTION, NULL, item->dict_size);
                dictionary_register_insert_callback(item->instance, item->dict_insert_cb, NULL);
            }
        }
        initialized = true;
    }

    for (int i = 0; hyperv_perf_list[i].registry_name != NULL; i++) {
        // Find the registry ID using the registry name
        DWORD id = RegistryFindIDByName(hyperv_perf_list[i].registry_name);
        if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
            continue;

        // Get the performance data using the registry ID
        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
        if (!pDataBlock)
            continue;

        hyperv_perf_list[i].function_collect(pDataBlock, update_every, &hyperv_perf_list[i]);
    }
    return 0;
}
