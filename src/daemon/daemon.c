// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include <sched.h>

char *pidfile = NULL;
char *netdata_exe_path = NULL;

void get_netdata_execution_path(void) {
    struct passwd *passwd = getpwuid(getuid());
    char *user = (passwd && passwd->pw_name) ? passwd->pw_name : "";

    char b[FILENAME_MAX + 1];
    size_t b_size = sizeof(b) - 1;
    int ret = uv_exepath(b, &b_size);
    if (ret != 0) {
        fatal("Cannot start netdata without getting execution path. "
            "(uv_exepath(\"%s\", %zu), user: '%s', failed: %s).",
            b, b_size, user, uv_strerror(ret));
    }
    b[b_size] = '\0';

    netdata_exe_path = strdupz(b);
}

static void fix_directory_file_permissions(const char *dirname, uid_t uid, gid_t gid, bool recursive)
{
    char filename[FILENAME_MAX + 1];

    DIR *dir = opendir(dirname);
    if (!dir)
        return;

    struct dirent *de = NULL;

    while ((de = readdir(dir))) {
        if (de->d_type == DT_DIR && (!strcmp(de->d_name, ".") || !strcmp(de->d_name, "..")))
            continue;

        (void) snprintfz(filename, FILENAME_MAX, "%s/%s", dirname, de->d_name);
        if (de->d_type == DT_REG || recursive) {
            if (chown(filename, uid, gid) == -1)
                netdata_log_error("Cannot chown %s '%s' to %u:%u", de->d_type == DT_DIR ? "directory" : "file", filename, (unsigned int)uid, (unsigned int)gid);
        }

        if (de->d_type == DT_DIR && recursive)
            fix_directory_file_permissions(filename, uid, gid, recursive);
    }

    closedir(dir);
}

static void change_dir_ownership(const char *dir, uid_t uid, gid_t gid, bool recursive)
{
    if (chown(dir, uid, gid) == -1)
        netdata_log_error("Cannot chown directory '%s' to %u:%u", dir, (unsigned int)uid, (unsigned int)gid);

    fix_directory_file_permissions(dir, uid, gid, recursive);
}

static inline void clean_directory(const char *dirname)
{
    DIR *dir = opendir(dirname);
    if(!dir) return;

    int dir_fd = dirfd(dir);
    struct dirent *de = NULL;

    while((de = readdir(dir)))
        if(de->d_type == DT_REG)
            if (unlinkat(dir_fd, de->d_name, 0))
                netdata_log_error("Cannot delete %s/%s", dirname, de->d_name);

    closedir(dir);
}

static void prepare_required_directories(uid_t uid, gid_t gid) {
    change_dir_ownership(os_run_dir(true), uid, gid, false);
    change_dir_ownership(netdata_configured_cache_dir, uid, gid, true);
    change_dir_ownership(netdata_configured_varlib_dir, uid, gid, false);
    change_dir_ownership(netdata_configured_log_dir, uid, gid, false);
    change_dir_ownership(netdata_configured_cloud_dir, uid, gid, false);

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/registry", netdata_configured_varlib_dir);
    change_dir_ownership(filename, uid, gid, false);
}

static int become_user(const char *username, int pid_fd) {
    int am_i_root = (getuid() == 0)?1:0;

    struct passwd *pw = getpwnam(username);
    if(!pw) {
        netdata_log_error("User %s is not present.", username);
        return -1;
    }

    uid_t uid = pw->pw_uid;
    gid_t gid = pw->pw_gid;

    prepare_required_directories(uid, gid);

    if(pidfile && *pidfile) {
        if(chown(pidfile, uid, gid) == -1)
            netdata_log_error("Cannot chown '%s' to %u:%u", pidfile, (unsigned int)uid, (unsigned int)gid);
    }

    int ngroups = (int)sysconf(_SC_NGROUPS_MAX);
    gid_t *supplementary_groups = NULL;
    if(ngroups > 0) {
        supplementary_groups = mallocz(sizeof(gid_t) * ngroups);
        if(os_getgrouplist(username, gid, supplementary_groups, &ngroups) == -1) {
            if(am_i_root)
                netdata_log_error("Cannot get supplementary groups of user '%s'.", username);

            ngroups = 0;
        }
    }

    nd_log_chown_log_files(uid, gid);
    chown_open_file(STDOUT_FILENO, uid, gid);
    chown_open_file(STDERR_FILENO, uid, gid);
    chown_open_file(pid_fd, uid, gid);

    if(supplementary_groups && ngroups > 0) {
        if(setgroups((size_t)ngroups, supplementary_groups) == -1) {
            if(am_i_root)
                netdata_log_error("Cannot set supplementary groups for user '%s'", username);
        }
        ngroups = 0;
    }

    if(supplementary_groups)
        freez(supplementary_groups);

    if(os_setresgid(gid, gid, gid) != 0) {
        netdata_log_error("Cannot switch to user's %s group (gid: %u).", username, gid);
        return -1;
    }

    if(os_setresuid(uid, uid, uid) != 0) {
        netdata_log_error("Cannot switch to user %s (uid: %u).", username, uid);
        return -1;
    }

    if(setgid(gid) != 0) {
        netdata_log_error("Cannot switch to user's %s group (gid: %u).", username, gid);
        return -1;
    }
    if(setegid(gid) != 0) {
        netdata_log_error("Cannot effectively switch to user's %s group (gid: %u).", username, gid);
        return -1;
    }
    if(setuid(uid) != 0) {
        netdata_log_error("Cannot switch to user %s (uid: %u).", username, uid);
        return -1;
    }
    if(seteuid(uid) != 0) {
        netdata_log_error("Cannot effectively switch to user %s (uid: %u).", username, uid);
        return -1;
    }

    return(0);
}

#ifndef OOM_SCORE_ADJ_MAX
#define OOM_SCORE_ADJ_MAX (1000)
#endif
#ifndef OOM_SCORE_ADJ_MIN
#define OOM_SCORE_ADJ_MIN (-1000)
#endif

static void oom_score_adj(void) {
    char buf[30 + 1];
    long long int old_score, wanted_score = 0, final_score = 0;

    // read the existing score
    if(read_single_signed_number_file("/proc/self/oom_score_adj", &old_score)) {
        netdata_log_error("Out-Of-Memory (OOM) score setting is not supported on this system.");
        return;
    }

    if (old_score != 0) {
        wanted_score = old_score;
        analytics_report_oom_score(old_score);
    }

    // check the environment
    const char *s = getenv("OOMScoreAdjust");
    if(!s || !*s) {
        snprintfz(buf, sizeof(buf) - 1, "%d", (int)wanted_score);
        s = buf;
    }

    // check netdata.conf configuration
    s = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "OOM score", s);
    if(s && *s && (isdigit((uint8_t)*s) || *s == '-' || *s == '+'))
        wanted_score = atoll(s);
    else if(s && !strcmp(s, "keep")) {
        netdata_log_info("Out-Of-Memory (OOM) kept as-is (running with %d)", (int) old_score);
        return;
    }
    else {
        netdata_log_info("Out-Of-Memory (OOM) score not changed due to non-numeric setting: '%s' (running with %d)", s, (int)old_score);
        return;
    }

    if(wanted_score < OOM_SCORE_ADJ_MIN) {
        netdata_log_error("Wanted Out-Of-Memory (OOM) score %d is too small. Using %d", (int)wanted_score, (int)OOM_SCORE_ADJ_MIN);
        wanted_score = OOM_SCORE_ADJ_MIN;
    }

    if(wanted_score > OOM_SCORE_ADJ_MAX) {
        netdata_log_error("Wanted Out-Of-Memory (OOM) score %d is too big. Using %d", (int)wanted_score, (int)OOM_SCORE_ADJ_MAX);
        wanted_score = OOM_SCORE_ADJ_MAX;
    }

    if(old_score == wanted_score) {
        netdata_log_info("Out-Of-Memory (OOM) score is already set to the wanted value %d", (int)old_score);
        return;
    }

    int written = 0;
    int fd = open("/proc/self/oom_score_adj", O_WRONLY | O_CLOEXEC);
    if(fd != -1) {
        snprintfz(buf, sizeof(buf) - 1, "%d", (int)wanted_score);
        ssize_t len = strlen(buf);
        if(len > 0 && write(fd, buf, (size_t)len) == len) written = 1;
        close(fd);

        if(written) {
            if(read_single_signed_number_file("/proc/self/oom_score_adj", &final_score))
                netdata_log_error("Adjusted my Out-Of-Memory (OOM) score to %d, but cannot verify it.", (int)wanted_score);
            else if(final_score == wanted_score)
                netdata_log_info("Adjusted my Out-Of-Memory (OOM) score from %d to %d.", (int)old_score, (int)final_score);
            else
                netdata_log_error("Adjusted my Out-Of-Memory (OOM) score from %d to %d, but it has been set to %d.", (int)old_score, (int)wanted_score, (int)final_score);
            analytics_report_oom_score(final_score);
        }
        else
            netdata_log_error("Failed to adjust my Out-Of-Memory (OOM) score to %d. Running with %d. (systemd systems may change it via netdata.service)", (int)wanted_score, (int)old_score);
    }
    else
        netdata_log_error("Failed to adjust my Out-Of-Memory (OOM) score. Cannot open /proc/self/oom_score_adj for writing.");
}

static void process_nice_level(void) {
#ifdef HAVE_NICE
    int nice_level = (int)inicfg_get_number(&netdata_config, CONFIG_SECTION_GLOBAL, "process nice level", 0);
    if(nice(nice_level) == -1)
        netdata_log_error("Cannot set netdata CPU nice level to %d.", nice_level);
    else
        netdata_log_debug(D_SYSTEM, "Set netdata nice level to %d.", nice_level);
#endif // HAVE_NICE
}

#define SCHED_FLAG_NONE                      0x00
#define SCHED_FLAG_PRIORITY_CONFIGURABLE     0x01 // the priority is user configurable
#define SCHED_FLAG_KEEP_AS_IS                0x04 // do not attempt to set policy, priority or nice()
#define SCHED_FLAG_USE_NICE                  0x08 // use nice() after setting this policy

struct sched_def {
    char *name;
    int policy;
    int priority;
    uint8_t flags;
} scheduler_defaults[] = {

        // the order of array members is important!
        // the first defined is the default used by netdata

        // the available members are important too!
        // these are all the possible scheduling policies supported by netdata

#ifdef SCHED_BATCH
        { "batch", SCHED_BATCH, 0, SCHED_FLAG_USE_NICE },
#endif

#ifdef SCHED_OTHER
        { "other", SCHED_OTHER, 0, SCHED_FLAG_USE_NICE },
        { "nice",  SCHED_OTHER, 0, SCHED_FLAG_USE_NICE },
#endif

#ifdef SCHED_IDLE
        { "idle", SCHED_IDLE, 0, SCHED_FLAG_NONE },
#endif

#ifdef SCHED_RR
        { "rr", SCHED_RR, 0, SCHED_FLAG_PRIORITY_CONFIGURABLE },
#endif

#ifdef SCHED_FIFO
        { "fifo", SCHED_FIFO, 0, SCHED_FLAG_PRIORITY_CONFIGURABLE },
#endif

        // do not change the scheduling priority
        { "keep", 0, 0, SCHED_FLAG_KEEP_AS_IS },
        { "none", 0, 0, SCHED_FLAG_KEEP_AS_IS },

        // array termination
        { NULL, 0, 0, 0 }
};


#ifdef HAVE_SCHED_GETSCHEDULER
static void sched_getscheduler_report(void) {
    int sched = sched_getscheduler(0);
    if(sched == -1) {
        netdata_log_error("Cannot get my current process scheduling policy.");
        return;
    }
    else {
        int i;
        for(i = 0 ; scheduler_defaults[i].name ; i++) {
            if(scheduler_defaults[i].policy == sched) {
                if(scheduler_defaults[i].flags & SCHED_FLAG_PRIORITY_CONFIGURABLE) {
                    struct sched_param param;
                    if(sched_getparam(0, &param) == -1) {
                        netdata_log_error("Cannot get the process scheduling priority for my policy '%s'", scheduler_defaults[i].name);
                        return;
                    }
                    else {
                        netdata_log_info("Running with process scheduling policy '%s', priority %d", scheduler_defaults[i].name, param.sched_priority);
                    }
                }
                else if(scheduler_defaults[i].flags & SCHED_FLAG_USE_NICE) {
                    #ifdef HAVE_GETPRIORITY
                    int n = getpriority(PRIO_PROCESS, 0);
                    netdata_log_info("Running with process scheduling policy '%s', nice level %d", scheduler_defaults[i].name, n);
                    #else // !HAVE_GETPRIORITY
                    netdata_log_info("Running with process scheduling policy '%s'", scheduler_defaults[i].name);
                    #endif // !HAVE_GETPRIORITY
                }
                else {
                    netdata_log_info("Running with process scheduling policy '%s'", scheduler_defaults[i].name);
                }

                return;
            }
        }
    }
}
#endif /* HAVE_SCHED_GETSCHEDULER */

#ifdef HAVE_SCHED_SETSCHEDULER

static void sched_setscheduler_set(void) {

    if(scheduler_defaults[0].name) {
        const char *name = scheduler_defaults[0].name;
        int policy = scheduler_defaults[0].policy, priority = scheduler_defaults[0].priority;
        uint8_t flags = scheduler_defaults[0].flags;
        int found = 0;

        // read the configuration
        name = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "process scheduling policy", name);
        int i;
        for(i = 0 ; scheduler_defaults[i].name ; i++) {
            if(!strcmp(name, scheduler_defaults[i].name)) {
                found = 1;
                policy = scheduler_defaults[i].policy;
                priority = scheduler_defaults[i].priority;
                flags = scheduler_defaults[i].flags;

                if(flags & SCHED_FLAG_KEEP_AS_IS)
                    goto report;

                if(flags & SCHED_FLAG_PRIORITY_CONFIGURABLE)
                    priority = (int)inicfg_get_number(&netdata_config, CONFIG_SECTION_GLOBAL, "process scheduling priority", priority);

#ifdef HAVE_SCHED_GET_PRIORITY_MIN
                errno_clear();
                if(priority < sched_get_priority_min(policy)) {
                    netdata_log_error("scheduler %s (%d) priority %d is below the minimum %d. Using the minimum.", name, policy, priority, sched_get_priority_min(policy));
                    priority = sched_get_priority_min(policy);
                }
#endif
#ifdef HAVE_SCHED_GET_PRIORITY_MAX
                errno_clear();
                if(priority > sched_get_priority_max(policy)) {
                    netdata_log_error("scheduler %s (%d) priority %d is above the maximum %d. Using the maximum.", name, policy, priority, sched_get_priority_max(policy));
                    priority = sched_get_priority_max(policy);
                }
#endif
                break;
            }
        }

        if(!found) {
            netdata_log_error("Unknown scheduling policy '%s' - falling back to nice", name);
            goto fallback;
        }

        const struct sched_param param = {
                .sched_priority = priority
        };

        errno_clear();
        i = sched_setscheduler(0, policy, &param);
        if(i != 0) {
            netdata_log_error("Cannot adjust netdata scheduling policy to %s (%d), with priority %d. Falling back to nice.",
                              name,
                              policy,
                              priority);
        }
        else {
            netdata_log_info("Adjusted netdata scheduling policy to %s (%d), with priority %d.", name, policy, priority);
            if(!(flags & SCHED_FLAG_USE_NICE))
                goto report;
        }
    }

fallback:
    process_nice_level();

report:
    sched_getscheduler_report();
}
#else /* HAVE_SCHED_SETSCHEDULER */
static void sched_setscheduler_set(void) {
    process_nice_level();
}
#endif /* HAVE_SCHED_SETSCHEDULER */

int become_daemon(int dont_fork, const char *user)
{
    if(!dont_fork) {
        int i = fork();
        if(i == -1) {
            perror("cannot fork");
            exit(1);
        }
        if(i != 0) exit(0); // the parent
        gettid_uncached();

        // become session leader
        if (setsid() < 0) {
            perror("Cannot become session leader.");
            exit(2);
        }

        // fork() again
        i = fork();
        if(i == -1) {
            perror("cannot fork");
            exit(1);
        }
        if(i != 0) exit(0); // the parent
        gettid_uncached();
    }

    // generate our pid file
    int pidfd = -1;
    if(pidfile && *pidfile) {
        pidfd = open(pidfile, O_WRONLY | O_CREAT | O_CLOEXEC, 0644);
        if(pidfd >= 0) {
            if(ftruncate(pidfd, 0) != 0)
                netdata_log_error("Cannot truncate pidfile '%s'.", pidfile);

            char b[100];
            sprintf(b, "%d\n", getpid());
            ssize_t i = write(pidfd, b, strlen(b));
            if(i <= 0)
                netdata_log_error("Cannot write pidfile '%s'.", pidfile);
        }
        else
            netdata_log_error("Failed to open pidfile '%s'.", pidfile);
    }

    // Set new file permissions
    umask(0007);

    // adjust my Out-Of-Memory score
    oom_score_adj();

    // never become a problem
    sched_setscheduler_set();

    if(user && *user) {
        if(become_user(user, pidfd) != 0) {
            netdata_log_error("Cannot become user '%s'. Continuing as we are.", user);
        }
        else
            netdata_log_debug(D_SYSTEM, "Successfully became user '%s'.", user);
    }
    else {
        prepare_required_directories(getuid(), getgid());
    }

    if(pidfd != -1)
        close(pidfd);

    return(0);
}
