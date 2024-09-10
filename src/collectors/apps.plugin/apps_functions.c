// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

bool enable_function_cmdline = false;

#define PROCESS_FILTER_CATEGORY "category:"
#define PROCESS_FILTER_USER "user:"
#define PROCESS_FILTER_GROUP "group:"
#define PROCESS_FILTER_PROCESS "process:"
#define PROCESS_FILTER_PID "pid:"
#define PROCESS_FILTER_UID "uid:"
#define PROCESS_FILTER_GID "gid:"

static void apps_plugin_function_processes_help(const char *transaction) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb, "%s",
                   "apps.plugin / processes\n"
                   "\n"
                   "Function `processes` presents all the currently running processes of the system.\n"
                   "\n"
                   "The following filters are supported:\n"
                   "\n"
                   "   category:NAME\n"
                   "      Shows only processes that are assigned the category `NAME` in apps_groups.conf\n"
                   "\n"
                   "   user:NAME\n"
                   "      Shows only processes that are running as user name `NAME`.\n"
                   "\n"
                   "   group:NAME\n"
                   "      Shows only processes that are running as group name `NAME`.\n"
                   "\n"
                   "   process:NAME\n"
                   "      Shows only processes that their Command is `NAME` or their parent's Command is `NAME`.\n"
                   "\n"
                   "   pid:NUMBER\n"
                   "      Shows only processes that their PID is `NUMBER` or their parent's PID is `NUMBER`\n"
                   "\n"
                   "   uid:NUMBER\n"
                   "      Shows only processes that their UID is `NUMBER`\n"
                   "\n"
                   "   gid:NUMBER\n"
                   "      Shows only processes that their GID is `NUMBER`\n"
                   "\n"
                   "Filters can be combined. Each filter can be given only one time.\n"
    );

    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_TEXT_PLAIN;
    wb->expires = now_realtime_sec() + 3600;
    pluginsd_function_result_to_stdout(transaction, wb);
    buffer_free(wb);
}

#define add_value_field_llu_with_max(wb, key, value) do {                                                       \
    unsigned long long _tmp = (value);                                                                          \
    key ## _max = (rows == 0) ? (_tmp) : MAX(key ## _max, _tmp);                                                \
    buffer_json_add_array_item_uint64(wb, _tmp);                                                                \
} while(0)

#define add_value_field_ndd_with_max(wb, key, value) do {                                                       \
    NETDATA_DOUBLE _tmp = (value);                                                                              \
    key ## _max = (rows == 0) ? (_tmp) : MAX(key ## _max, _tmp);                                                \
    buffer_json_add_array_item_double(wb, _tmp);                                                                \
} while(0)

void function_processes(const char *transaction, char *function,
                               usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
                               BUFFER *payload __maybe_unused, HTTP_ACCESS access,
                               const char *source __maybe_unused, void *data __maybe_unused) {
    time_t now_s = now_realtime_sec();
    struct pid_stat *p;

    bool show_cmdline = http_access_user_has_enough_access_level_for_endpoint(
                            access,
                            HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA |
                                HTTP_ACCESS_VIEW_AGENT_CONFIG) || enable_function_cmdline;

    char *words[PLUGINSD_MAX_WORDS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, PLUGINSD_MAX_WORDS);

    struct target *category = NULL, *user = NULL, *group = NULL;
    const char *process_name = NULL;
    pid_t pid = 0;
    uid_t uid = 0;
    gid_t gid = 0;
    bool info = false;

    bool filter_pid = false, filter_uid = false, filter_gid = false;

    for(int i = 1; i < PLUGINSD_MAX_WORDS ;i++) {
        const char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(!category && strncmp(keyword, PROCESS_FILTER_CATEGORY, strlen(PROCESS_FILTER_CATEGORY)) == 0) {
            category = find_target_by_name(apps_groups_root_target, &keyword[strlen(PROCESS_FILTER_CATEGORY)]);
            if(!category) {
                pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_BAD_REQUEST,
                                                       "No category with that name found.");
                return;
            }
        }
        else if(!user && strncmp(keyword, PROCESS_FILTER_USER, strlen(PROCESS_FILTER_USER)) == 0) {
            user = find_target_by_name(users_root_target, &keyword[strlen(PROCESS_FILTER_USER)]);
            if(!user) {
                pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_BAD_REQUEST,
                                                       "No user with that name found.");
                return;
            }
        }
        else if(strncmp(keyword, PROCESS_FILTER_GROUP, strlen(PROCESS_FILTER_GROUP)) == 0) {
            group = find_target_by_name(groups_root_target, &keyword[strlen(PROCESS_FILTER_GROUP)]);
            if(!group) {
                pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_BAD_REQUEST,
                                                       "No group with that name found.");
                return;
            }
        }
        else if(!process_name && strncmp(keyword, PROCESS_FILTER_PROCESS, strlen(PROCESS_FILTER_PROCESS)) == 0) {
            process_name = &keyword[strlen(PROCESS_FILTER_PROCESS)];
        }
        else if(!pid && strncmp(keyword, PROCESS_FILTER_PID, strlen(PROCESS_FILTER_PID)) == 0) {
            pid = str2i(&keyword[strlen(PROCESS_FILTER_PID)]);
            filter_pid = true;
        }
        else if(!uid && strncmp(keyword, PROCESS_FILTER_UID, strlen(PROCESS_FILTER_UID)) == 0) {
            uid = str2i(&keyword[strlen(PROCESS_FILTER_UID)]);
            filter_uid = true;
        }
        else if(!gid && strncmp(keyword, PROCESS_FILTER_GID, strlen(PROCESS_FILTER_GID)) == 0) {
            gid = str2i(&keyword[strlen(PROCESS_FILTER_GID)]);
            filter_gid = true;
        }
        else if(strcmp(keyword, "help") == 0) {
            apps_plugin_function_processes_help(transaction);
            return;
        }
        else if(strcmp(keyword, "info") == 0) {
            info = true;
        }
    }

    unsigned int cpu_divisor = time_factor * RATES_DETAIL / 100;
    unsigned int memory_divisor = 1024;
    unsigned int io_divisor = 1024 * RATES_DETAIL;

    BUFFER *wb = buffer_create(4096, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", update_every);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", APPS_PLUGIN_PROCESSES_FUNCTION_DESCRIPTION);
    buffer_json_member_add_array(wb, "data");

    if(info)
        goto close_and_send;

    NETDATA_DOUBLE
    UserCPU_max = 0.0
        , SysCPU_max = 0.0
        , GuestCPU_max = 0.0
        , CUserCPU_max = 0.0
        , CSysCPU_max = 0.0
        , CGuestCPU_max = 0.0
        , CPU_max = 0.0
        , VMSize_max = 0.0
        , RSS_max = 0.0
        , Shared_max = 0.0
        , Swap_max = 0.0
        , Memory_max = 0.0
        , FDsLimitPercent_max = 0.0
        ;

    unsigned long long
        Processes_max = 0
        , Threads_max = 0
        , VoluntaryCtxtSwitches_max = 0
        , NonVoluntaryCtxtSwitches_max = 0
        , Uptime_max = 0
        , MinFlt_max = 0
        , CMinFlt_max = 0
        , TMinFlt_max = 0
        , MajFlt_max = 0
        , CMajFlt_max = 0
        , TMajFlt_max = 0
        , PReads_max = 0
        , PWrites_max = 0
        , RCalls_max = 0
        , WCalls_max = 0
        , Files_max = 0
        , Pipes_max = 0
        , Sockets_max = 0
        , iNotiFDs_max = 0
        , EventFDs_max = 0
        , TimerFDs_max = 0
        , SigFDs_max = 0
        , EvPollFDs_max = 0
        , OtherFDs_max = 0
        , FDs_max = 0
        ;

#if !defined(__FreeBSD__) && !defined(__APPLE__)
    unsigned long long
        LReads_max = 0
        , LWrites_max = 0
        ;
#endif // !__FreeBSD__ !__APPLE_

    int rows= 0;
    for(p = root_of_pids; p ; p = p->next) {
        if(!p->updated)
            continue;

        if(category && p->target != category)
            continue;

        if(user && p->user_target != user)
            continue;

        if(group && p->group_target != group)
            continue;

        if(process_name && ((strcmp(p->comm, process_name) != 0 && !p->parent) || (p->parent && strcmp(p->comm, process_name) != 0 && strcmp(p->parent->comm, process_name) != 0)))
            continue;

        if(filter_pid && p->pid != pid && p->ppid != pid)
            continue;

        if(filter_uid && p->uid != uid)
            continue;

        if(filter_gid && p->gid != gid)
            continue;

        rows++;

        buffer_json_add_array_item_array(wb); // for each pid

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE FIELDS!

        // pid
        buffer_json_add_array_item_uint64(wb, p->pid);

        // cmd
        buffer_json_add_array_item_string(wb, p->comm);

        // cmdline
        if (show_cmdline) {
            buffer_json_add_array_item_string(wb, (p->cmdline && *p->cmdline) ? p->cmdline : p->comm);
        }

        // ppid
        buffer_json_add_array_item_uint64(wb, p->ppid);

        // category
        buffer_json_add_array_item_string(wb, p->target ? p->target->name : "-");

        // user
        buffer_json_add_array_item_string(wb, p->user_target ? p->user_target->name : "-");

        // uid
        buffer_json_add_array_item_uint64(wb, p->uid);

        // group
        buffer_json_add_array_item_string(wb, p->group_target ? p->group_target->name : "-");

        // gid
        buffer_json_add_array_item_uint64(wb, p->gid);

        // CPU utilization %
        add_value_field_ndd_with_max(wb, CPU, (NETDATA_DOUBLE)(p->utime + p->stime + p->gtime + p->cutime + p->cstime + p->cgtime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, UserCPU, (NETDATA_DOUBLE)(p->utime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, SysCPU, (NETDATA_DOUBLE)(p->stime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, GuestCPU, (NETDATA_DOUBLE)(p->gtime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, CUserCPU, (NETDATA_DOUBLE)(p->cutime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, CSysCPU, (NETDATA_DOUBLE)(p->cstime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, CGuestCPU, (NETDATA_DOUBLE)(p->cgtime) / cpu_divisor);

        add_value_field_llu_with_max(wb, VoluntaryCtxtSwitches, p->status_voluntary_ctxt_switches / RATES_DETAIL);
        add_value_field_llu_with_max(wb, NonVoluntaryCtxtSwitches, p->status_nonvoluntary_ctxt_switches / RATES_DETAIL);

        // memory MiB
        if(MemTotal)
            add_value_field_ndd_with_max(wb, Memory, (NETDATA_DOUBLE)p->status_vmrss * 100.0 / (NETDATA_DOUBLE)MemTotal);

        add_value_field_ndd_with_max(wb, RSS, (NETDATA_DOUBLE)p->status_vmrss / memory_divisor);
        add_value_field_ndd_with_max(wb, Shared, (NETDATA_DOUBLE)p->status_vmshared / memory_divisor);
#if !defined(__APPLE__)
        add_value_field_ndd_with_max(wb, VMSize, (NETDATA_DOUBLE)p->status_vmsize / memory_divisor);
#endif
        add_value_field_ndd_with_max(wb, Swap, (NETDATA_DOUBLE)p->status_vmswap / memory_divisor);

        // Physical I/O
        add_value_field_llu_with_max(wb, PReads, p->io_storage_bytes_read / io_divisor);
        add_value_field_llu_with_max(wb, PWrites, p->io_storage_bytes_written / io_divisor);

        // Logical I/O
#if !defined(__FreeBSD__) && !defined(__APPLE__)
        add_value_field_llu_with_max(wb, LReads, p->io_logical_bytes_read / io_divisor);
        add_value_field_llu_with_max(wb, LWrites, p->io_logical_bytes_written / io_divisor);
#endif

        // I/O calls
        add_value_field_llu_with_max(wb, RCalls, p->io_read_calls / RATES_DETAIL);
        add_value_field_llu_with_max(wb, WCalls, p->io_write_calls / RATES_DETAIL);

        // minor page faults
        add_value_field_llu_with_max(wb, MinFlt, p->minflt / RATES_DETAIL);
        add_value_field_llu_with_max(wb, CMinFlt, p->cminflt / RATES_DETAIL);
        add_value_field_llu_with_max(wb, TMinFlt, (p->minflt + p->cminflt) / RATES_DETAIL);

        // major page faults
        add_value_field_llu_with_max(wb, MajFlt, p->majflt / RATES_DETAIL);
        add_value_field_llu_with_max(wb, CMajFlt, p->cmajflt / RATES_DETAIL);
        add_value_field_llu_with_max(wb, TMajFlt, (p->majflt + p->cmajflt) / RATES_DETAIL);

        // open file descriptors
        add_value_field_ndd_with_max(wb, FDsLimitPercent, p->openfds_limits_percent);
        add_value_field_llu_with_max(wb, FDs, pid_openfds_sum(p));
        add_value_field_llu_with_max(wb, Files, p->openfds.files);
        add_value_field_llu_with_max(wb, Pipes, p->openfds.pipes);
        add_value_field_llu_with_max(wb, Sockets, p->openfds.sockets);
        add_value_field_llu_with_max(wb, iNotiFDs, p->openfds.inotifies);
        add_value_field_llu_with_max(wb, EventFDs, p->openfds.eventfds);
        add_value_field_llu_with_max(wb, TimerFDs, p->openfds.timerfds);
        add_value_field_llu_with_max(wb, SigFDs, p->openfds.signalfds);
        add_value_field_llu_with_max(wb, EvPollFDs, p->openfds.eventpolls);
        add_value_field_llu_with_max(wb, OtherFDs, p->openfds.other);


        // processes, threads, uptime
        add_value_field_llu_with_max(wb, Processes, p->children_count);
        add_value_field_llu_with_max(wb, Threads, p->num_threads);
        add_value_field_llu_with_max(wb, Uptime, p->uptime);

        buffer_json_array_close(wb); // for each pid
    }

    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");

    {
        int field_id = 0;

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE VALUES!
        // wb, key, name, visible, type, visualization, transform, decimal_points, units, max, sort, sortable, sticky, unique_key, pointer_to, summary, range
        buffer_rrdf_table_add_field(wb, field_id++, "PID", "Process ID", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY |
                                        RRDF_FIELD_OPTS_UNIQUE_KEY, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Cmd", "Process Name", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        if (show_cmdline) {
            buffer_rrdf_table_add_field(wb, field_id++, "CmdLine", "Command Line", RRDF_FIELD_TYPE_STRING,
                                        RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0,
                                        NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                        RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_NONE, NULL);
        }

        buffer_rrdf_table_add_field(wb, field_id++, "PPID", "Parent Process ID", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL,
                                    NAN, RRDF_FIELD_SORT_ASCENDING, "PID", RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Category", "Category (apps_groups.conf)", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE,
                                    RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "User", "User Owner", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Uid", "User ID", RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Group", "Group Owner", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Gid", "Group ID", RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // CPU utilization
        buffer_rrdf_table_add_field(wb, field_id++, "CPU", "Total CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", CPU_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "UserCPU", "User CPU time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", UserCPU_max,
                                    RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "SysCPU", "System CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", SysCPU_max,
                                    RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "GuestCPU", "Guest CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", GuestCPU_max,
                                    RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CUserCPU", "Children User CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", CUserCPU_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CSysCPU", "Children System CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", CSysCPU_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CGuestCPU", "Children Guest CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", CGuestCPU_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_NONE, NULL);

        // CPU context switches
        buffer_rrdf_table_add_field(wb, field_id++, "vCtxSwitch", "Voluntary Context Switches",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "switches/s",
                                    VoluntaryCtxtSwitches_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "iCtxSwitch", "Involuntary Context Switches",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "switches/s",
                                    NonVoluntaryCtxtSwitches_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_NONE, NULL);

        // memory
        if (MemTotal)
            buffer_rrdf_table_add_field(wb, field_id++, "Memory", "Memory Percentage", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                        RRDF_FIELD_VISUAL_BAR,
                                        RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                        RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Resident", "Resident Set Size", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "MiB", RSS_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Shared", "Shared Pages", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2,
                                    "MiB", Shared_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
#if !defined(__APPLE__)
        buffer_rrdf_table_add_field(wb, field_id++, "Virtual", "Virtual Memory Size", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "MiB", VMSize_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
#endif
        buffer_rrdf_table_add_field(wb, field_id++, "Swap", "Swap Memory", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2,
                                    "MiB",
                                    Swap_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // Physical I/O
        buffer_rrdf_table_add_field(wb, field_id++, "PReads", "Physical I/O Reads", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "KiB/s", PReads_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "PWrites", "Physical I/O Writes", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "KiB/s", PWrites_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // Logical I/O
#if !defined(__FreeBSD__) && !defined(__APPLE__)
        buffer_rrdf_table_add_field(wb, field_id++, "LReads", "Logical I/O Reads", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "KiB/s", LReads_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "LWrites", "Logical I/O Writes", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "KiB/s", LWrites_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
#endif

        // I/O calls
        buffer_rrdf_table_add_field(wb, field_id++, "RCalls", "I/O Read Calls", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2,
                                    "calls/s", RCalls_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "WCalls", "I/O Write Calls", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2,
                                    "calls/s", WCalls_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // minor page faults
        buffer_rrdf_table_add_field(wb, field_id++, "MinFlt", "Minor Page Faults/s", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "pgflts/s", MinFlt_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CMinFlt", "Children Minor Page Faults/s",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "pgflts/s", CMinFlt_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "TMinFlt", "Total Minor Page Faults/s",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "pgflts/s", TMinFlt_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // major page faults
        buffer_rrdf_table_add_field(wb, field_id++, "MajFlt", "Major Page Faults/s", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "pgflts/s", MajFlt_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CMajFlt", "Children Major Page Faults/s",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "pgflts/s", CMajFlt_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "TMajFlt", "Total Major Page Faults/s",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "pgflts/s", TMajFlt_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // open file descriptors
        buffer_rrdf_table_add_field(wb, field_id++, "FDsLimitPercent", "Percentage of Open Descriptors vs Limits",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", FDsLimitPercent_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "FDs", "All Open File Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", FDs_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Files", "Open Files", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "fds",
                                    Files_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Pipes", "Open Pipes", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "fds",
                                    Pipes_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Sockets", "Open Sockets", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "fds", Sockets_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "iNotiFDs", "Open iNotify Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", iNotiFDs_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "EventFDs", "Open Event Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", EventFDs_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "TimerFDs", "Open Timer Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", TimerFDs_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "SigFDs", "Open Signal Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", SigFDs_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "EvPollFDs", "Open Event Poll Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", EvPollFDs_max,
                                    RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "OtherFDs", "Other Open Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", OtherFDs_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // processes, threads, uptime
        buffer_rrdf_table_add_field(wb, field_id++, "Processes", "Processes", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "processes", Processes_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Threads", "Threads", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "threads", Threads_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Uptime", "Uptime in seconds", RRDF_FIELD_TYPE_DURATION,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_DURATION_S, 2,
                                    "seconds", Uptime_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_MAX,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "CPU");

    buffer_json_member_add_object(wb, "charts");
    {
        // CPU chart
        buffer_json_member_add_object(wb, "CPU");
        {
            buffer_json_member_add_string(wb, "name", "CPU Utilization");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "UserCPU");
                buffer_json_add_array_item_string(wb, "SysCPU");
                buffer_json_add_array_item_string(wb, "GuestCPU");
                buffer_json_add_array_item_string(wb, "CUserCPU");
                buffer_json_add_array_item_string(wb, "CSysCPU");
                buffer_json_add_array_item_string(wb, "CGuestCPU");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "CPUCtxSwitches");
        {
            buffer_json_member_add_string(wb, "name", "CPU Context Switches");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "vCtxSwitch");
                buffer_json_add_array_item_string(wb, "iCtxSwitch");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Memory chart
        buffer_json_member_add_object(wb, "Memory");
        {
            buffer_json_member_add_string(wb, "name", "Memory");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Virtual");
                buffer_json_add_array_item_string(wb, "Resident");
                buffer_json_add_array_item_string(wb, "Shared");
                buffer_json_add_array_item_string(wb, "Swap");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        if(MemTotal) {
            // Memory chart
            buffer_json_member_add_object(wb, "MemoryPercent");
            {
                buffer_json_member_add_string(wb, "name", "Memory Percentage");
                buffer_json_member_add_string(wb, "type", "stacked-bar");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Memory");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
        }

#if !defined(__FreeBSD__) && !defined(__APPLE__)
        // I/O Reads chart
        buffer_json_member_add_object(wb, "Reads");
        {
            buffer_json_member_add_string(wb, "name", "I/O Reads");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "LReads");
                buffer_json_add_array_item_string(wb, "PReads");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // I/O Writes chart
        buffer_json_member_add_object(wb, "Writes");
        {
            buffer_json_member_add_string(wb, "name", "I/O Writes");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "LWrites");
                buffer_json_add_array_item_string(wb, "PWrites");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Logical I/O chart
        buffer_json_member_add_object(wb, "LogicalIO");
        {
            buffer_json_member_add_string(wb, "name", "Logical I/O");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "LReads");
                buffer_json_add_array_item_string(wb, "LWrites");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
#endif

        // Physical I/O chart
        buffer_json_member_add_object(wb, "PhysicalIO");
        {
            buffer_json_member_add_string(wb, "name", "Physical I/O");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "PReads");
                buffer_json_add_array_item_string(wb, "PWrites");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // I/O Calls chart
        buffer_json_member_add_object(wb, "IOCalls");
        {
            buffer_json_member_add_string(wb, "name", "I/O Calls");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "RCalls");
                buffer_json_add_array_item_string(wb, "WCalls");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Minor Page Faults chart
        buffer_json_member_add_object(wb, "MinFlt");
        {
            buffer_json_member_add_string(wb, "name", "Minor Page Faults");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "MinFlt");
                buffer_json_add_array_item_string(wb, "CMinFlt");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Major Page Faults chart
        buffer_json_member_add_object(wb, "MajFlt");
        {
            buffer_json_member_add_string(wb, "name", "Major Page Faults");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "MajFlt");
                buffer_json_add_array_item_string(wb, "CMajFlt");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Threads chart
        buffer_json_member_add_object(wb, "Threads");
        {
            buffer_json_member_add_string(wb, "name", "Threads");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Threads");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Processes chart
        buffer_json_member_add_object(wb, "Processes");
        {
            buffer_json_member_add_string(wb, "name", "Processes");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Processes");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // FDs chart
        buffer_json_member_add_object(wb, "FDs");
        {
            buffer_json_member_add_string(wb, "name", "File Descriptors");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Files");
                buffer_json_add_array_item_string(wb, "Pipes");
                buffer_json_add_array_item_string(wb, "Sockets");
                buffer_json_add_array_item_string(wb, "iNotiFDs");
                buffer_json_add_array_item_string(wb, "EventFDs");
                buffer_json_add_array_item_string(wb, "TimerFDs");
                buffer_json_add_array_item_string(wb, "SigFDs");
                buffer_json_add_array_item_string(wb, "EvPollFDs");
                buffer_json_add_array_item_string(wb, "OtherFDs");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "CPU");
        buffer_json_add_array_item_string(wb, "Category");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Memory");
        buffer_json_add_array_item_string(wb, "Category");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        // group by PID
        buffer_json_member_add_object(wb, "PID");
        {
            buffer_json_member_add_string(wb, "name", "Process Tree by PID");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "PPID");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by Category
        buffer_json_member_add_object(wb, "Category");
        {
            buffer_json_member_add_string(wb, "name", "Process Tree by Category");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Category");
                buffer_json_add_array_item_string(wb, "PPID");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by User
        buffer_json_member_add_object(wb, "User");
        {
            buffer_json_member_add_string(wb, "name", "Process Tree by User");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "User");
                buffer_json_add_array_item_string(wb, "PPID");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by Group
        buffer_json_member_add_object(wb, "Group");
        {
            buffer_json_member_add_string(wb, "name", "Process Tree by Group");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Group");
                buffer_json_add_array_item_string(wb, "PPID");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

close_and_send:
    buffer_json_member_add_time_t(wb, "expires", now_s + update_every);
    buffer_json_finalize(wb);

    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + update_every;
    pluginsd_function_result_to_stdout(transaction, wb);

    buffer_free(wb);
}
