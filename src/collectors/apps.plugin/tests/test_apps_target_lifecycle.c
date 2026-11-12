// SPDX-License-Identifier: GPL-3.0-or-later
//
// Dynamic target lifecycle: admission cap, overflow identity, unique chart
// names and retirement ownership. Links the real apps_targets.c against stubs
// for the process list and the chart output.

#include "../apps_plugin.h"

bool debug_enabled = false;
bool proc_pid_cmdline_is_needed = false;
bool enable_users_charts = true;
bool enable_groups_charts = true;
size_t global_iterations_counter = 1;

static struct pid_stat *test_pids = NULL;
struct pid_stat *root_of_pids(void) {
    return test_pids;
}

#define MAX_OBSOLETED 16
static const char *obsoleted[MAX_OBSOLETED];
static size_t obsoleted_count = 0;
void send_target_charts_obsolete(struct target *w, const struct target_family *f __maybe_unused) {
    if(obsoleted_count < MAX_OBSOLETED)
        obsoleted[obsoleted_count++] = strdupz(string2str(w->clean_name));
    w->exposed = false;
}

#include "../apps_pid_match.c"
#include "../apps_targets.c"

static int failures = 0;
#define CHECK(cond, ...) do { if(!(cond)) { fprintf(stderr, "FAIL %s:%d: ", __FILE__, __LINE__); fprintf(stderr, __VA_ARGS__); fputc('\n', stderr); failures++; } } while(0)

#define MAX_TEST_PIDS 16
static struct pid_stat *allocated_pids[MAX_TEST_PIDS];
static size_t allocated_pids_count = 0;

static struct pid_stat *test_pid(pid_t pid, const char *comm) {
    struct pid_stat *p = callocz(1, sizeof(*p));
    if(allocated_pids_count < MAX_TEST_PIDS)
        allocated_pids[allocated_pids_count++] = p;
    p->pid = pid;
    p->ppid = INIT_PID;
    p->comm = string_strdupz(comm);
    p->comm_orig = string_dup(p->comm);
    return p;
}

static void set_pids(struct pid_stat **pids, size_t n) {
    test_pids = NULL;
    for(size_t i = n; i > 0 ; i--) {
        pids[i - 1]->next = test_pids;
        test_pids = pids[i - 1];
    }
}

static bool in_list(struct target *root, struct target *w) {
    for(struct target *t = root; t ; t = t->next)
        if(t == w) return true;
    return false;
}

// a new iteration: retirement considers what the process list references now
static void iterate(void) {
    global_iterations_counter++;
    for(size_t i = 0; i < obsoleted_count ; i++)
        freez((void *)obsoleted[i]);
    obsoleted_count = 0;
    apps_targets_mark_referenced();
    apps_targets_retire();
}

static void test_uid_admission_overflow_and_retirement(void) {
    max_targets_per_family = 3;
    retire_targets_after_s = 0;   // idle for one iteration is enough here

    struct target *t10 = get_uid_target(10), *t11 = get_uid_target(11), *t12 = get_uid_target(12);
    CHECK(t10 && t11 && t12 && t10 != t11 && t11 != t12, "distinct uid targets");
    CHECK(target_families[TARGET_FAMILY_UID].dynamic == 3, "3 dynamic uid targets, got %zu", target_families[TARGET_FAMILY_UID].dynamic);
    CHECK(target_families[TARGET_FAMILY_UID].generation == 3, "each real target bumps the generation, got %zu", target_families[TARGET_FAMILY_UID].generation);

    struct target *o = get_uid_target(0);
    CHECK(o && o->overflow, "uid 0 at the cap goes to the overflow target");
    CHECK(get_uid_target(13) == o, "every refused uid shares one overflow target");
    CHECK(o && strcmp(string2str(o->name), "other") == 0, "overflow target is named other");
    CHECK(target_families[TARGET_FAMILY_UID].dynamic == 3, "overflow is not counted as dynamic");
    CHECK(target_families[TARGET_FAMILY_UID].generation == 3, "the overflow target does not bump the generation");

    // only uid 10 and uid 11 are still referenced; uid 11 only through a group-less process
    struct pid_stat *p1 = test_pid(100, "p1"), *p2 = test_pid(101, "p2");
    p1->uid_target = t10;
    p2->uid_target = t11;
    struct pid_stat *pids[] = { p1, p2 };
    set_pids(pids, 2);

    t12->exposed = true;
    t12->charts = TARGET_CHARTS_BASE;
    o->exposed = false;

    iterate();

    CHECK(in_list(users_root_target, t10) && in_list(users_root_target, t11), "referenced targets survive");
    CHECK(!in_list(users_root_target, t12), "idle uid 12 is retired");
    CHECK(target_families[TARGET_FAMILY_UID].overflow == NULL, "idle overflow target is retired");
    CHECK(target_families[TARGET_FAMILY_UID].dynamic == 2, "dynamic count follows retirement, got %zu", target_families[TARGET_FAMILY_UID].dynamic);
    CHECK(target_families[TARGET_FAMILY_UID].retired == 2, "two targets retired, got %zu", target_families[TARGET_FAMILY_UID].retired);
    CHECK(target_families[TARGET_FAMILY_UID].generation == 4, "retiring a real target bumps the generation, the overflow does not, got %zu", target_families[TARGET_FAMILY_UID].generation);
    CHECK(obsoleted_count == 1, "only the exposed target emits obsoletion, got %zu", obsoleted_count);

    // below the cap again: uid 0 gets its own target, it is not captured by a zeroed overflow
    struct target *t0 = get_uid_target(0);
    CHECK(t0 && !t0->overflow && t0->uid == 0, "uid 0 gets a real target once a slot is free");

    test_pids = NULL;
    iterate();
    CHECK(users_root_target == NULL, "every unreferenced uid target is retired");
    CHECK(target_families[TARGET_FAMILY_UID].dynamic == 0, "dynamic count back to 0");
}

static void test_retirement_waits_for_the_idle_window(void) {
    max_targets_per_family = 10;
    retire_targets_after_s = 3600;

    struct target *g = get_gid_target(500);
    test_pids = NULL;
    iterate();
    CHECK(in_list(groups_root_target, g), "a target idle less than the window survives");

    // simulate the window passing
    g->last_active_ut -= 3601 * USEC_PER_SEC;
    iterate();
    CHECK(!in_list(groups_root_target, g), "a target idle longer than the window is retired");
}

static void test_tree_unique_names_cap_and_app_groups(void) {
    max_targets_per_family = 3;
    retire_targets_after_s = 0;

    // an apps_groups.conf target is permanent and does not count against the cap
    struct target *grp = callocz(1, sizeof(*grp));
    grp->type = TARGET_TYPE_APP_GROUP;
    grp->id = string_strdupz("grp");
    grp->name = string_strdupz("grp");
    grp->clean_name = string_strdupz("grp");
    grp->exposed = true;
    grp->next = apps_groups_root_target;
    apps_groups_root_target = grp;

    struct pid_stat *a = test_pid(200, "a.b"), *b = test_pid(201, "a_b"), *c = test_pid(202, "other");
    struct target *ta = get_tree_target(a), *tb = get_tree_target(b), *tc = get_tree_target(c);
    CHECK(ta != tb, "distinct identities, distinct targets");
    CHECK(strcmp(string2str(ta->clean_name), "a_b") == 0, "first keeps its clean name, got %s", string2str(ta->clean_name));
    CHECK(strcmp(string2str(tb->clean_name), "a_b_2") == 0, "colliding clean name is made unique, got %s", string2str(tb->clean_name));
    CHECK(tc && !tc->overflow, "a process called other gets its own target below the cap");
    CHECK(get_tree_target(a) == ta, "lookup finds the existing target");

    struct pid_stat *d = test_pid(203, "d");
    struct target *td = get_tree_target(d);
    CHECK(td && td->overflow, "the 4th tree identity goes to overflow");
    CHECK(td && strcmp(string2str(td->clean_name), "other_2") == 0, "overflow does not share a chart id with the real 'other', got %s", td ? string2str(td->clean_name) : "(null)");

    struct pid_stat *e = test_pid(204, "other");
    CHECK(get_tree_target(e) == tc, "a process called other matches the real target, not overflow");
    CHECK(find_target_by_name(apps_groups_root_target, "other") == tc, "a Function filter for other selects the real target, not overflow");

    test_pids = NULL;
    iterate();
    CHECK(in_list(apps_groups_root_target, grp), "apps_groups.conf target is never retired");
    CHECK(apps_groups_root_target == grp && grp->next == NULL, "every tree target is retired, the group stays");
    CHECK(target_families[TARGET_FAMILY_APPS].dynamic == 0, "no dynamic tree targets left");
}

int main(void) {
    cached_usernames_init();
    cached_groupnames_init();

    test_uid_admission_overflow_and_retirement();
    test_retirement_waits_for_the_idle_window();
    test_tree_unique_names_cap_and_app_groups();

    for(size_t i = 0; i < obsoleted_count ; i++)
        freez((void *)obsoleted[i]);

    for(size_t i = 0; i < allocated_pids_count ; i++) {
        string_freez(allocated_pids[i]->comm);
        string_freez(allocated_pids[i]->comm_orig);
        freez(allocated_pids[i]);
    }

    if(failures) {
        fprintf(stderr, "%d check(s) failed\n", failures);
        return 1;
    }

    fprintf(stderr, "apps target lifecycle: all checks passed\n");
    return 0;
}
