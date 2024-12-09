// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events.h"

//struct {
//    const char *name;
//    const wchar_t *query;
//} custom_queries[] = {
//    {
//        .name = "All-Administrative-Events",
//        .query = L"<QueryList>\n"
//                 "  <Query Id=\"0\" Path=\"Application\">\n"
//                 "    <Select Path=\"Application\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Security\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"System\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"HardwareEvents\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Internet Explorer\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Key Management Service\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-AppV-Client/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-AppV-Client/Virtual Applications\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-All-User-Install-Agent/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-AppHost/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Application Server-Applications/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-AppModel-Runtime/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-AppReadiness/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-AssignedAccess/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-AssignedAccessBroker/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Storage-ATAPort/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-BitLocker-DrivePreparationTool/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Client-Licensing-Platform/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-DataIntegrityScan/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-DataIntegrityScan/CrashRecovery\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-DSC/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-DeviceManagement-Enterprise-Diagnostics-Provider/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-DeviceManagement-Enterprise-Diagnostics-Provider/Autopilot\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-DeviceSetupManager/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Dhcp-Client/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Dhcpv6-Client/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Diagnosis-Scripted/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Storage-Disk/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-DxgKrnl-Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-EDP-Application-Learning/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-EDP-Audit-Regular/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-EDP-Audit-TCB/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Client-License-Flexible-Platform/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-GenericRoaming/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Hyper-V-Guest-Drivers/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Hyper-V-Hypervisor-Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Hyper-V-VID-Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Kernel-EventTracing/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-KeyboardFilter/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-ModernDeployment-Diagnostics-Provider/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-ModernDeployment-Diagnostics-Provider/Autopilot\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-ModernDeployment-Diagnostics-Provider/Diagnostics\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-ModernDeployment-Diagnostics-Provider/ManagementService\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-MUI/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-PowerShell/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-PrintBRM/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-PrintService/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Provisioning-Diagnostics-Provider/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Provisioning-Diagnostics-Provider/AutoPilot\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Provisioning-Diagnostics-Provider/ManagementService\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-PushNotification-Platform/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-RemoteApp and Desktop Connections/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-RemoteAssistance/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-RemoteDesktopServices-RdpCoreTS/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-RetailDemo/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-SecurityMitigationsBroker/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-SmartCard-TPM-VCard-Module/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-SMBDirect/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-SMBWitnessClient/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Storage-Tiering/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Storage-ClassPnP/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Storage-Storport/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-ClientUSBDevices/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-LocalSessionManager/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-PnPDevices/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-Printers/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-RemoteConnectionManager/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-ServerUSBDevices/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Troubleshooting-Recommended/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-User Device Registration/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-VerifyHardwareSecurity/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-WindowsBackup/ActionCenter\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-Workplace Join/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"OAlerts\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"OneApp_IGCC\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"OpenSSH/Admin\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"USER_ESRV_SVC_QUEENCREEK\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Visual Studio\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "    <Select Path=\"Windows PowerShell\">*[System[(Level=1  or Level=2 or Level=3)]]</Select>\n"
//                 "  </Query>\n"
//                 "</QueryList>",
//    },
//    {
//        .name = "All-Remote-Desktop-Services",
//        .query = L"<QueryList>\n"
//                 "  <Query Id=\"0\" Path=\"Microsoft-Rdms-UI/Admin\">\n"
//                 "    <Select Path=\"Microsoft-Rdms-UI/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Rdms-UI/Operational\">*</Select>\n"
//                 "    <Select Path=\"Remote-Desktop-Management-Service/Admin\">*</Select>\n"
//                 "    <Select Path=\"Remote-Desktop-Management-Service/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-SessionBroker-Client/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-SessionBroker-Client/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-RemoteConnectionManager/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-RemoteConnectionManager/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-PnPDevices/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-PnPDevices/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-RemoteApp and Desktop Connections/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-RemoteApp and Desktop Connection Management/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-RemoteApp and Desktop Connection Management/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-SessionBroker/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-SessionBroker/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-TSV-VmHostAgent/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-TSV-VmHostAgent/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-ServerUSBDevices/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-ServerUSBDevices/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-LocalSessionManager/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-LocalSessionManager/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-ClientUSBDevices/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-ClientUSBDevices/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-RDPClient/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-Licensing/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-Licensing/Operational\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-Gateway/Admin\">*</Select>\n"
//                 "    <Select Path=\"Microsoft-Windows-TerminalServices-Gateway/Operational\">*</Select>\n"
//                 "  </Query>\n"
//                 "</QueryList>",
//    },
//    {
//        .name = "All-Security-SPP",
//        .query = L"<QueryList>\n"
//                 "  <Query Id=\"0\" Path=\"Microsoft-Windows-HelloForBusiness/Operational\">\n"
//                 "    <Select Path=\"Microsoft-Windows-HelloForBusiness/Operational\">*[System[(Level&gt;5 )]]</Select>\n"
//                 "  </Query>\n"
//                 "</QueryList>",
//    }
//};

ENUM_STR_MAP_DEFINE(WEVT_SOURCE_TYPE) = {
    { .id = WEVTS_ALL,                      .name = WEVT_SOURCE_ALL_NAME },
    { .id = WEVTS_ADMIN,                    .name = WEVT_SOURCE_ALL_ADMIN_NAME },
    { .id = WEVTS_OPERATIONAL,              .name = WEVT_SOURCE_ALL_OPERATIONAL_NAME },
    { .id = WEVTS_ANALYTIC,                 .name = WEVT_SOURCE_ALL_ANALYTIC_NAME },
    { .id = WEVTS_DEBUG,                    .name = WEVT_SOURCE_ALL_DEBUG_NAME },
    { .id = WEVTS_WINDOWS,                  .name = WEVT_SOURCE_ALL_WINDOWS_NAME },
    { .id = WEVTS_ENABLED,                  .name = WEVT_SOURCE_ALL_ENABLED_NAME },
    { .id = WEVTS_DISABLED,                 .name = WEVT_SOURCE_ALL_DISABLED_NAME },
    { .id = WEVTS_FORWARDED,                .name = WEVT_SOURCE_ALL_FORWARDED_NAME },
    { .id = WEVTS_CLASSIC,                  .name = WEVT_SOURCE_ALL_CLASSIC_NAME },
    { .id = WEVTS_BACKUP_MODE,              .name = WEVT_SOURCE_ALL_BACKUP_MODE_NAME },
    { .id = WEVTS_OVERWRITE_MODE,           .name = WEVT_SOURCE_ALL_OVERWRITE_MODE_NAME },
    { .id = WEVTS_STOP_WHEN_FULL_MODE,      .name = WEVT_SOURCE_ALL_STOP_WHEN_FULL_MODE_NAME },
    { .id = WEVTS_RETAIN_AND_BACKUP_MODE,   .name = WEVT_SOURCE_ALL_RETAIN_AND_BACKUP_MODE_NAME },

    // terminator
    { . id = 0, .name = NULL }
};

BITMAP_STR_DEFINE_FUNCTIONS(WEVT_SOURCE_TYPE, WEVTS_NONE, "");

DICTIONARY *wevt_sources = NULL;
DICTIONARY *used_hashes_registry = NULL;
static usec_t wevt_session = 0;

void wevt_sources_del_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    LOGS_QUERY_SOURCE *src = value;
    freez((void *)src->fullname);
    string_freez(src->source);

    src->fullname = NULL;
    src->source = NULL;
}

static bool wevt_sources_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    LOGS_QUERY_SOURCE *src_old = old_value;
    LOGS_QUERY_SOURCE *src_new = new_value;

    bool ret = false;
    if(src_new->last_scan_monotonic_ut > src_old->last_scan_monotonic_ut) {
        src_old->last_scan_monotonic_ut = src_new->last_scan_monotonic_ut;

        if (src_old->source != src_new->source) {
            string_freez(src_old->source);
            src_old->source = src_new->source;
            src_new->source = NULL;
        }
        src_old->source_type = src_new->source_type;

        src_old->msg_first_ut = src_new->msg_first_ut;
        src_old->msg_last_ut = src_new->msg_last_ut;
        src_old->msg_first_id = src_new->msg_first_id;
        src_old->msg_last_id = src_new->msg_last_id;
        src_old->entries = src_new->entries;
        src_old->size = src_new->size;

        ret = true;
    }

    freez((void *)src_new->fullname);
    string_freez(src_new->source);
    src_new->fullname = NULL;
    src_new->source = NULL;

    return ret;
}

void wevt_sources_init(void) {
    wevt_session = now_realtime_usec();

    used_hashes_registry = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);

    wevt_sources = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE,
                                              NULL, sizeof(LOGS_QUERY_SOURCE));

    dictionary_register_delete_callback(wevt_sources, wevt_sources_del_cb, NULL);
    dictionary_register_conflict_callback(wevt_sources, wevt_sources_conflict_cb, NULL);
}

void buffer_json_wevt_versions(BUFFER *wb __maybe_unused) {
    buffer_json_member_add_object(wb, "versions");
    {
        buffer_json_member_add_uint64(wb, "sources",
                wevt_session + dictionary_version(wevt_sources));
    }
    buffer_json_object_close(wb);
}

// --------------------------------------------------------------------------------------------------------------------

int wevt_sources_dict_items_backward_compar(const void *a, const void *b) {
    const DICTIONARY_ITEM **da = (const DICTIONARY_ITEM **)a, **db = (const DICTIONARY_ITEM **)b;
    LOGS_QUERY_SOURCE *sa = dictionary_acquired_item_value(*da);
    LOGS_QUERY_SOURCE *sb = dictionary_acquired_item_value(*db);

    // compare the last message timestamps
    if(sa->msg_last_ut < sb->msg_last_ut)
        return 1;

    if(sa->msg_last_ut > sb->msg_last_ut)
        return -1;

    // compare the first message timestamps
    if(sa->msg_first_ut < sb->msg_first_ut)
        return 1;

    if(sa->msg_first_ut > sb->msg_first_ut)
        return -1;

    return 0;
}

int wevt_sources_dict_items_forward_compar(const void *a, const void *b) {
    return -wevt_sources_dict_items_backward_compar(a, b);
}

// --------------------------------------------------------------------------------------------------------------------

typedef enum {
        wevt_source_type_internal,
        wevt_source_type_provider,
        wevt_source_type_channel,
} wevt_source_type;

struct wevt_source {
    wevt_source_type type;
    usec_t first_ut;
    usec_t last_ut;
    size_t count;
    size_t entries;
    uint64_t size;
};

static int wevt_source_to_json_array_cb(const DICTIONARY_ITEM *item, void *entry, void *data) {
    const struct wevt_source *s = entry;
    BUFFER *wb = data;

    const char *name = dictionary_acquired_item_name(item);

    if(s->count == 1 && strncmp(name, WEVT_SOURCE_ALL_OF_PROVIDER_PREFIX, sizeof(WEVT_SOURCE_ALL_OF_PROVIDER_PREFIX) - 1) == 0)
        // do not include "All-Of-X" when there is only 1 channel
        return 0;

    bool default_selected = (s->type == wevt_source_type_channel);
    if(default_selected && (strcmp(name, "NetdataWEL") == 0 || strcmp(name, "Netdata/Access") == 0))
        // do not select Netdata Access logs by default
        default_selected = false;

    buffer_json_add_array_item_object(wb);
    {
        char size_for_humans[128];
        size_snprintf(size_for_humans, sizeof(size_for_humans), s->size, "B", false);

        char duration_for_humans[128];
        duration_snprintf(duration_for_humans, sizeof(duration_for_humans),
                          (time_t)((s->last_ut - s->first_ut) / USEC_PER_SEC), "s", true);

        char entries_for_humans[128];
        entries_snprintf(entries_for_humans, sizeof(entries_for_humans), s->entries, "", false);

        char info[1024];
        snprintfz(info, sizeof(info), "%zu channel%s, with a total size of %s, covering %s%s%s%s",
                s->count, s->count > 1 ? "s":"", size_for_humans, duration_for_humans,
                s->entries ? ", having " : "", s->entries ? entries_for_humans : "", s->entries ? " entries" : "");

        buffer_json_member_add_string(wb, "id", name);
        buffer_json_member_add_string(wb, "name", name);
        buffer_json_member_add_string(wb, "pill", size_for_humans);
        buffer_json_member_add_string(wb, "info", info);
        buffer_json_member_add_boolean(wb, "default_selected", default_selected);
    }
    buffer_json_object_close(wb); // options object

    return 1;
}

static bool wevt_source_merge_sizes(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value , void *data __maybe_unused) {
    struct wevt_source *old_v = old_value;
    const struct wevt_source *new_v = new_value;

    old_v->count += new_v->count;
    old_v->size += new_v->size;
    old_v->entries += new_v->entries;

    if(new_v->first_ut && new_v->first_ut < old_v->first_ut)
        old_v->first_ut = new_v->first_ut;

    if(new_v->last_ut && new_v->last_ut > old_v->last_ut)
        old_v->last_ut = new_v->last_ut;

    return false;
}

void wevt_sources_to_json_array(BUFFER *wb) {
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_NAME_LINK_DONT_CLONE|DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_conflict_callback(dict, wevt_source_merge_sizes, NULL);

    struct wevt_source t = { 0 };

    LOGS_QUERY_SOURCE *src;
    dfe_start_read(wevt_sources, src) {
        t.first_ut = src->msg_first_ut;
        t.last_ut = src->msg_last_ut;
        t.count = 1;
        t.size = src->size;
        t.entries = src->entries;

        src->source_type |= WEVTS_ALL;
        t.type = wevt_source_type_internal;
        for(size_t i = 0; WEVT_SOURCE_TYPE_names[i].name ;i++) {
            if(src->source_type & WEVT_SOURCE_TYPE_names[i].id)
                dictionary_set(dict, WEVT_SOURCE_TYPE_names[i].name, &t, sizeof(t));
        }

        if(src->provider) {
            t.type = wevt_source_type_provider;
            dictionary_set(dict, string2str(src->provider), &t, sizeof(t));
        }

        if(src->source) {
            t.type = wevt_source_type_channel;
            dictionary_set(dict, string2str(src->source), &t, sizeof(t));
        }
    }
    dfe_done(jf);

    dictionary_sorted_walkthrough_read(dict, wevt_source_to_json_array_cb, wb);
}

static bool ndEvtGetChannelConfigProperty(EVT_HANDLE hChannelConfig, WEVT_VARIANT *pr, EVT_CHANNEL_CONFIG_PROPERTY_ID id) {
    if (!EvtGetChannelConfigProperty(hChannelConfig, id, 0, pr->size, pr->data, &pr->used)) {
        DWORD status = GetLastError();
        if (ERROR_INSUFFICIENT_BUFFER == status) {
            wevt_variant_resize(pr, pr->used);
            if(!EvtGetChannelConfigProperty(hChannelConfig, id, 0, pr->size, pr->data, &pr->used)) {
                pr->used = 0;
                pr->count = 0;
                return false;
            }
        }
    }

    wevt_variant_count_from_used(pr);
    return true;
}

WEVT_SOURCE_TYPE categorize_channel(const wchar_t *channel_path, const char **provider, WEVT_VARIANT *property) {
    EVT_HANDLE hChannelConfig = NULL;
    WEVT_SOURCE_TYPE result = WEVTS_ALL;

    // Open the channel configuration
    hChannelConfig = EvtOpenChannelConfig(NULL, channel_path, 0);
    if (!hChannelConfig)
        goto cleanup;

    if(ndEvtGetChannelConfigProperty(hChannelConfig, property, EvtChannelConfigType) &
       property->count &&
       property->data[0].Type == EvtVarTypeUInt32) {
        switch (property->data[0].UInt32Val) {
            case EvtChannelTypeAdmin:
                result |= WEVTS_ADMIN;
                break;

            case EvtChannelTypeOperational:
                result |= WEVTS_OPERATIONAL;
                break;

            case EvtChannelTypeAnalytic:
                result |= WEVTS_ANALYTIC;
                break;

            case EvtChannelTypeDebug:
                result |= WEVTS_DEBUG;
                break;

            default:
                break;
        }
    }

    if(ndEvtGetChannelConfigProperty(hChannelConfig, property, EvtChannelConfigClassicEventlog) &&
       property->count &&
       property->data[0].Type == EvtVarTypeBoolean &&
       property->data[0].BooleanVal)
        result |= WEVTS_CLASSIC;

    if(ndEvtGetChannelConfigProperty(hChannelConfig, property, EvtChannelConfigOwningPublisher) &&
       property->count &&
       property->data[0].Type == EvtVarTypeString) {
        *provider = provider2utf8(property->data[0].StringVal);
        if(wcscasecmp(property->data[0].StringVal, L"Microsoft-Windows-EventCollector") == 0)
            result |= WEVTS_FORWARDED;
    }
    else
        *provider = NULL;

    if(ndEvtGetChannelConfigProperty(hChannelConfig, property, EvtChannelConfigEnabled) &&
       property->count &&
       property->data[0].Type == EvtVarTypeBoolean) {
        if(property->data[0].BooleanVal)
            result |= WEVTS_ENABLED;
        else
            result |= WEVTS_DISABLED;
    }

    bool got_retention = false;
    bool retained = false;
    if(ndEvtGetChannelConfigProperty(hChannelConfig, property, EvtChannelLoggingConfigRetention) &&
       property->count &&
       property->data[0].Type == EvtVarTypeBoolean) {
        got_retention = true;
        retained = property->data[0].BooleanVal;
    }

    bool got_auto_backup = false;
    bool auto_backup = false;
    if(ndEvtGetChannelConfigProperty(hChannelConfig, property, EvtChannelLoggingConfigAutoBackup) &&
       property->count &&
       property->data[0].Type == EvtVarTypeBoolean) {
        got_auto_backup = true;
        auto_backup = property->data[0].BooleanVal;
    }

    if(got_retention && got_auto_backup) {
        if(!retained) {
            if(auto_backup)
                result |= WEVTS_BACKUP_MODE;
            else
                result |= WEVTS_OVERWRITE_MODE;
        }
        else {
            if(auto_backup)
                result |= WEVTS_STOP_WHEN_FULL_MODE;
            else
                result |= WEVTS_RETAIN_AND_BACKUP_MODE;
        }
    }

cleanup:
    if (hChannelConfig)
        EvtClose(hChannelConfig);

    return result;
}

void wevt_sources_scan(void) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    LPWSTR channel = NULL;
    EVT_HANDLE hChannelEnum = NULL;

    if(spinlock_trylock(&spinlock)) {
        const usec_t started_ut = now_monotonic_usec();

        WEVT_VARIANT property = { 0 };
        DWORD dwChannelBufferSize = 0;
        DWORD dwChannelBufferUsed = 0;
        DWORD status = ERROR_SUCCESS;

        // Open a handle to enumerate the event channels
        hChannelEnum = EvtOpenChannelEnum(NULL, 0);
        if (!hChannelEnum) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS EVENTS: EvtOpenChannelEnum() failed with %" PRIu64 "\n",
                    (uint64_t)GetLastError());
            goto cleanup;
        }

        WEVT_LOG *log = wevt_openlog6(WEVT_QUERY_RETENTION);
        if(!log) goto cleanup;

        while (true) {
            if (!EvtNextChannelPath(hChannelEnum, dwChannelBufferSize, channel, &dwChannelBufferUsed)) {
                status = GetLastError();
                if (status == ERROR_NO_MORE_ITEMS)
                    break; // No more channels
                else if (status == ERROR_INSUFFICIENT_BUFFER) {
                    dwChannelBufferSize = dwChannelBufferUsed;
                    freez(channel);
                    channel = mallocz(dwChannelBufferSize * sizeof(WCHAR));
                    continue;
                } else {
                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "WINDOWS EVENTS: EvtNextChannelPath() failed\n");
                    break;
                }
            }

            EVT_RETENTION retention;
            if(!wevt_channel_retention(log, channel, NULL, &retention))
                continue;

            LOGS_QUERY_SOURCE *found = dictionary_get(wevt_sources, channel2utf8(channel));
            if(found) {
                // we just need to update its retention

                found->last_scan_monotonic_ut = now_monotonic_usec();
                found->msg_first_id = retention.first_event.id;
                found->msg_last_id = retention.last_event.id;
                found->msg_first_ut = retention.first_event.created_ns / NSEC_PER_USEC;
                found->msg_last_ut = retention.last_event.created_ns / NSEC_PER_USEC;
                found->size = retention.size_bytes;
                continue;
            }

            const char *name = channel2utf8(channel);
            const char *fullname = strdupz(name);
            const char *provider;

            WEVT_SOURCE_TYPE sources = categorize_channel(channel, &provider, &property);
            char *slash = strchr(name, '/');
            if(slash) *slash = '\0';

            if(strcasecmp(name, "Application") == 0)
                sources |= WEVTS_WINDOWS;
            if(strcasecmp(name, "Security") == 0)
                sources |= WEVTS_WINDOWS;
            if(strcasecmp(name, "Setup") == 0)
                sources |= WEVTS_WINDOWS;
            if(strcasecmp(name, "System") == 0)
                sources |= WEVTS_WINDOWS;

            LOGS_QUERY_SOURCE src = {
                .entries = retention.entries,
                .fullname = fullname,
                .fullname_len = strlen(fullname),
                .last_scan_monotonic_ut = now_monotonic_usec(),
                .msg_first_id = retention.first_event.id,
                .msg_last_id = retention.last_event.id,
                .msg_first_ut = retention.first_event.created_ns / NSEC_PER_USEC,
                .msg_last_ut = retention.last_event.created_ns / NSEC_PER_USEC,
                .size = retention.size_bytes,
                .source_type = sources,
                .source = string_strdupz(fullname),
            };

            if(strncmp(fullname, "Netdata", 7) == 0)
                // WEL based providers of Netdata are named NetdataX
                provider = "Netdata";

            if(provider && *provider) {
                char buf[sizeof(WEVT_SOURCE_ALL_OF_PROVIDER_PREFIX) + strlen(provider)]; // sizeof() includes terminator
                snprintf(buf, sizeof(buf), WEVT_SOURCE_ALL_OF_PROVIDER_PREFIX "%s", provider);

                if(trim_all(buf) != NULL) {
                    for (size_t i = 0; i < sizeof(buf) - 1; i++) {
                        // remove character that may interfere with our parsing
                        if (isspace((uint8_t) buf[i]) || buf[i] == '%' || buf[i] == '+' || buf[i] == '|' || buf[i] == ':')
                            buf[i] = '_';
                    }
                    src.provider = string_strdupz(buf);
                }
            }

            dictionary_set(wevt_sources, src.fullname, &src, sizeof(src));
        }

//        // add custom queries
//        for(size_t i = 0; i < sizeof(custom_queries) / sizeof(custom_queries[0]) ;i++) {
//            EVT_RETENTION retention;
//            if(!wevt_channel_retention(log, NULL, custom_queries[i].query, &retention))
//                continue;
//
//            LOGS_QUERY_SOURCE src = {
//                    .entries = 0,
//                    .fullname = strdupz(custom_queries[i].name),
//                    .fullname_len = strlen(custom_queries[i].name),
//                    .last_scan_monotonic_ut = now_monotonic_usec(),
//                    .msg_first_id = retention.first_event.id,
//                    .msg_last_id = retention.last_event.id,
//                    .msg_first_ut = retention.first_event.created_ns / NSEC_PER_USEC,
//                    .msg_last_ut = retention.last_event.created_ns / NSEC_PER_USEC,
//                    .size = retention.size_bytes,
//                    .source_type = WEVTS_ALL,
//                    .source = string_strdupz(custom_queries[i].name),
//            };
//
//            dictionary_set(wevt_sources, src.fullname, &src, sizeof(src));
//        }
//
        wevt_closelog6(log);

        LOGS_QUERY_SOURCE *src;
        dfe_start_write(wevt_sources, src)
        {
            if(src->last_scan_monotonic_ut < started_ut) {
                src->msg_first_id = 0;
                src->msg_last_id = 0;
                src->msg_first_ut = 0;
                src->msg_last_ut = 0;
                src->size = 0;
                dictionary_del(wevt_sources, src->fullname);
            }
        }
        dfe_done(src);
        dictionary_garbage_collect(wevt_sources);

        spinlock_unlock(&spinlock);

        wevt_variant_cleanup(&property);
    }

cleanup:
    freez(channel);
    EvtClose(hChannelEnum);
}
