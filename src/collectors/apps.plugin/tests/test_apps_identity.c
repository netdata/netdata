// SPDX-License-Identifier: GPL-3.0-or-later

#include "../apps_pid.c"
#include "../apps_pid_match.c"
#include "../apps_targets.c"
#include "../apps_aggregations.c"
#include "../apps_incremental_collection.c"

#define main apps_plugin_main
#include "../apps_plugin.c"
#undef main

static const char *os_cmdline;
static size_t cmdline_reads;

bool apps_os_get_pid_cmdline_linux(struct pid_stat *p __maybe_unused, char *buf, size_t size) {
    cmdline_reads++;
    if(!os_cmdline) {
        errno = ENOENT;
        return false;
    }
    snprintf(buf, size, "%s", os_cmdline);
    return true;
}

#define CHECK(condition) do { if(!(condition)) { \
    fprintf(stderr, "%s:%d: %s\n", __func__, __LINE__, #condition); return false; \
} } while(0)

static void clear_identity(struct pid_stat *p) {
    string_freez(p->comm_orig);
    string_freez(p->comm);
    string_freez(p->cmdline);
}

static bool test_refresh_schedule(void) {
    struct pid_stat p = {.pid = 60};
    struct target assigned = {0};
    global_iterations_counter = 1;
    cmdline_reads = 0;
    os_cmdline = "/bin/launcher";
    update_pid_comm(&p, "launcher");
    CHECK(cmdline_reads == 1);
    p.target = &assigned;

    global_iterations_counter = 2;
    os_cmdline = "/bin/clickhouse-server --new";
    update_pid_comm(&p, "clickhouse-serv");
    CHECK(cmdline_reads == 2);
    CHECK(strcmp(string2str(p.cmdline), os_cmdline) == 0);
    CHECK(strcmp(string2str(p.comm), "clickhouse-server") == 0);
    CHECK(!p.target && p.identity_changed && !p.matched_by_config);

    p.target = &assigned;
    p.identity_changed = false;
    p.matched_by_config = true;
    for(global_iterations_counter = 3; global_iterations_counter <= 60; global_iterations_counter++)
        update_pid_comm(&p, "clickhouse-serv");
    CHECK(cmdline_reads == 4);
    CHECK(p.target == &assigned && !p.identity_changed && p.matched_by_config);

    os_cmdline = "/bin/other --new";
    global_iterations_counter = 90;
    update_pid_comm(&p, "clickhouse-serv");
    CHECK(cmdline_reads == 5 && !p.target);
    CHECK(strcmp(string2str(p.comm), "clickhouse-serv") == 0);
    CHECK(!p.matched_by_config);

    // A failed refresh clears stale matching and retries on the next scheduled iteration.
    os_cmdline = NULL;
    p.target = &assigned;
    global_iterations_counter = 120;
    update_pid_comm(&p, "clickhouse-serv");
    CHECK(!p.cmdline && !p.target);
    os_cmdline = "/bin/clickhouse-server --recovered";
    for(global_iterations_counter = 121; global_iterations_counter <= 150; global_iterations_counter++)
        update_pid_comm(&p, "clickhouse-serv");
    CHECK(cmdline_reads == 7);
    CHECK(strcmp(string2str(p.cmdline), os_cmdline) == 0);
    clear_identity(&p);
    return true;
}

static bool test_staggering_and_initial_failure(void) {
    struct pid_stat processes[30] = {0};
    global_iterations_counter = 1;
    os_cmdline = NULL;
    for(size_t i = 0; i < 30; i++) {
        processes[i].pid = 300 + i;
        update_pid_comm(&processes[i], "worker");
    }
    os_cmdline = "/bin/worker";
    for(global_iterations_counter = 2; global_iterations_counter <= 31; global_iterations_counter++) {
        cmdline_reads = 0;
        for(size_t i = 0; i < 30; i++)
            update_pid_comm(&processes[i], "worker");
        CHECK(cmdline_reads == 1);
    }
    for(size_t i = 0; i < 30; i++) {
        CHECK(strcmp(string2str(processes[i].cmdline), os_cmdline) == 0);
        clear_identity(&processes[i]);
    }
    return true;
}

static bool test_empty_identity(void) {
    struct pid_stat p = {.pid = 60};
    struct target assigned = {0};
    os_cmdline = "";
    cmdline_reads = 0;
    global_iterations_counter = 1;
    update_pid_comm(&p, "");
    p.target = &assigned;
    p.identity_changed = false;
    for(global_iterations_counter = 2; global_iterations_counter <= 31; global_iterations_counter++)
        update_pid_comm(&p, "");
    CHECK(cmdline_reads == 2);
    CHECK(p.target == &assigned && !p.identity_changed);
    clear_identity(&p);
    return true;
}

static bool test_derived_metadata(void) {
    struct pid_stat p = {.pid = 600};
    global_iterations_counter = 1;
    os_cmdline = "python /proc/self/stat";
    update_pid_comm(&p, "python");
    CHECK(strcmp(string2str(p.comm), "stat") == 0);
    update_pid_cmdline(&p, "python /proc/self/status");
    CHECK(strcmp(string2str(p.comm), "status") == 0);
    managed_list_add(&tree.managers, "*--manager*");
    managed_list_add(&tree.aggregators, "*--aggregate*");
    update_pid_cmdline(&p, "python /proc/self/status --manager --aggregate");
    CHECK(p.is_manager && p.is_aggregator);
    update_pid_cmdline(&p, "python /proc/self/status");
    CHECK(!p.is_manager && !p.is_aggregator);
    clear_identity(&p);

    // Windows supplies these fields in the opposite order.
    struct pid_stat early = {0};
    update_pid_cmdline(&early, "/bin/worker");
    CHECK(!early.comm_orig && !early.comm);
    proc_pid_cmdline_is_needed = false;
    update_pid_comm(&early, "worker");
    CHECK(strcmp(string2str(early.comm), "worker") == 0);
    proc_pid_cmdline_is_needed = true;
    clear_identity(&early);
    return true;
}

static bool test_inherited_assignments(void) {
    struct target *old = get_apps_groups_target("*--old*", NULL, "old");
    struct target *new = get_apps_groups_target("*--new*", NULL, "new");
    struct target *explicit = get_apps_groups_target("explicit", NULL, "explicit");
    struct pid_stat parent = {.pid = 100, .ppid = 1};
    struct pid_stat child = {.pid = 101, .ppid = 100, .parent = &parent};
    struct pid_stat grandchild = {.pid = 102, .ppid = 101, .parent = &child};
    struct pid_stat direct = {.pid = 103, .ppid = 100, .parent = &parent};
    struct pid_stat orphan = {.pid = 104, .ppid = 1};
    struct pid_stat *all[] = {&grandchild, &child, &direct, &orphan, &parent};
    global_iterations_counter = 1;
    os_cmdline = "/bin/worker --old";
    update_pid_comm(&parent, "worker");
    for(size_t i = 0; i < 4; i++) {
        os_cmdline = "unmatched";
        update_pid_comm(all[i], all[i] == &direct ? "explicit" : "child");
        all[i]->next = all[i + 1];
    }
    pids.all_pids.root = all[0];
    assign_a_target_to_all_processes();
    CHECK(parent.target == old && child.target == old && grandchild.target == old);
    CHECK(direct.target == explicit);
    orphan.target = old;

    update_pid_cmdline(&parent, "/bin/worker --new");
    assign_a_target_to_all_processes();
    CHECK(parent.target == new && child.target == new && grandchild.target == new);
    CHECK(direct.target == explicit && orphan.target == old);

    update_pid_cmdline(&parent, "/bin/worker --none");
    assign_a_target_to_all_processes();
    CHECK(!parent.matched_by_config && parent.target != old && parent.target != new);
    CHECK(child.target == parent.target && grandchild.target == parent.target);

    update_pid_cmdline(&parent, "/bin/worker --manager");
    assign_a_target_to_all_processes();
    CHECK(parent.is_manager && child.target != parent.target && grandchild.target == child.target);
    CHECK(direct.target == explicit && orphan.target == old);
    update_pid_cmdline(&parent, "/bin/worker --new");
    assign_a_target_to_all_processes();
    CHECK(!parent.is_manager && child.target == new && grandchild.target == new);

    size_t matches = targets_assignment_counter;
    update_pid_cmdline(&parent, "/bin/worker --new");
    assign_a_target_to_all_processes();
    CHECK(targets_assignment_counter == matches);
    for(size_t i = 0; i < 5; i++) {
        CHECK(!all[i]->identity_changed);
        clear_identity(all[i]);
    }
    pids.all_pids.root = NULL;
    return true;
}

int main(void) {
    apps_managers_and_aggregators_init();
    if(!test_refresh_schedule() || !test_staggering_and_initial_failure() ||
       !test_empty_identity() || !test_derived_metadata() || !test_inherited_assignments())
        return 1;

    managed_list_clear(&tree.managers);
    managed_list_clear(&tree.aggregators);
    managed_list_clear(&tree.interpreters);
    string_freez(KernelAggregator);
    while(apps_groups_root_target) {
        struct target *t = apps_groups_root_target;
        apps_groups_root_target = t->next;
        string_freez(t->id);
        string_freez(t->name);
        string_freez(t->clean_name);
        pid_match_cleanup(&t->match);
        freez(t);
    }
    puts("apps identity tests passed");
    return 0;
}
