#include "common.h"
#include <sched.h>

char pidfile[FILENAME_MAX + 1] = "";

static void chown_open_file(int fd, uid_t uid, gid_t gid) {
    if(fd == -1) return;

    struct stat buf;

    if(fstat(fd, &buf) == -1) {
        error("Cannot fstat() fd %d", fd);
        return;
    }

    if((buf.st_uid != uid || buf.st_gid != gid) && S_ISREG(buf.st_mode)) {
        if(fchown(fd, uid, gid) == -1)
            error("Cannot fchown() fd %d.", fd);
    }
}

void create_needed_dir(const char *dir, uid_t uid, gid_t gid)
{
    // attempt to create the directory
    if(mkdir(dir, 0755) == 0) {
        // we created it

        // chown it to match the required user
        if(chown(dir, uid, gid) == -1)
            error("Cannot chown directory '%s' to %u:%u", dir, (unsigned int)uid, (unsigned int)gid);
    }
    else if(errno != EEXIST)
        // log an error only if the directory does not exist
        error("Cannot create directory '%s'", dir);
}

int become_user(const char *username, int pid_fd) {
    int am_i_root = (getuid() == 0)?1:0;

    struct passwd *pw = getpwnam(username);
    if(!pw) {
        error("User %s is not present.", username);
        return -1;
    }

    uid_t uid = pw->pw_uid;
    gid_t gid = pw->pw_gid;

    create_needed_dir(netdata_configured_cache_dir, uid, gid);
    create_needed_dir(netdata_configured_varlib_dir, uid, gid);

    if(pidfile[0]) {
        if(chown(pidfile, uid, gid) == -1)
            error("Cannot chown '%s' to %u:%u", pidfile, (unsigned int)uid, (unsigned int)gid);
    }

    int ngroups = (int)sysconf(_SC_NGROUPS_MAX);
    gid_t *supplementary_groups = NULL;
    if(ngroups > 0) {
        supplementary_groups = mallocz(sizeof(gid_t) * ngroups);
        if(getgrouplist(username, gid, supplementary_groups, &ngroups) == -1) {
            if(am_i_root)
                error("Cannot get supplementary groups of user '%s'.", username);

            ngroups = 0;
        }
    }

    chown_open_file(STDOUT_FILENO, uid, gid);
    chown_open_file(STDERR_FILENO, uid, gid);
    chown_open_file(stdaccess_fd, uid, gid);
    chown_open_file(pid_fd, uid, gid);

    if(supplementary_groups && ngroups > 0) {
        if(setgroups((size_t)ngroups, supplementary_groups) == -1) {
            if(am_i_root)
                error("Cannot set supplementary groups for user '%s'", username);
        }
        ngroups = 0;
    }

    if(supplementary_groups)
        freez(supplementary_groups);

#ifdef __APPLE__
    if(setregid(gid, gid) != 0) {
#else
    if(setresgid(gid, gid, gid) != 0) {
#endif /* __APPLE__ */
        error("Cannot switch to user's %s group (gid: %u).", username, gid);
        return -1;
    }

#ifdef __APPLE__
    if(setreuid(uid, uid) != 0) {
#else
    if(setresuid(uid, uid, uid) != 0) {
#endif /* __APPLE__ */
        error("Cannot switch to user %s (uid: %u).", username, uid);
        return -1;
    }

    if(setgid(gid) != 0) {
        error("Cannot switch to user's %s group (gid: %u).", username, gid);
        return -1;
    }
    if(setegid(gid) != 0) {
        error("Cannot effectively switch to user's %s group (gid: %u).", username, gid);
        return -1;
    }
    if(setuid(uid) != 0) {
        error("Cannot switch to user %s (uid: %u).", username, uid);
        return -1;
    }
    if(seteuid(uid) != 0) {
        error("Cannot effectively switch to user %s (uid: %u).", username, uid);
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
    long long int old_score, wanted_score = OOM_SCORE_ADJ_MAX, final_score = 0;

    // read the existing score
    if(read_single_signed_number_file("/proc/self/oom_score_adj", &old_score)) {
        error("Out-Of-Memory (OOM) score setting is not supported on this system.");
        return;
    }

    if(old_score != 0)
        wanted_score = old_score;

    // check the environment
    char *s = getenv("OOMScoreAdjust");
    if(!s || !*s) {
        snprintfz(buf, 30, "%d", (int)wanted_score);
        s = buf;
    }

    // check netdata.conf configuration
    s = config_get(CONFIG_SECTION_GLOBAL, "OOM score", s);
    if(s && *s && (isdigit(*s) || *s == '-' || *s == '+'))
        wanted_score = atoll(s);
    else {
        info("Out-Of-Memory (OOM) score not changed due to non-numeric setting: '%s' (running with %d)", s, (int)old_score);
        return;
    }

    if(wanted_score < OOM_SCORE_ADJ_MIN) {
        error("Wanted Out-Of-Memory (OOM) score %d is too small. Using %d", (int)wanted_score, (int)OOM_SCORE_ADJ_MIN);
        wanted_score = OOM_SCORE_ADJ_MIN;
    }

    if(wanted_score > OOM_SCORE_ADJ_MAX) {
        error("Wanted Out-Of-Memory (OOM) score %d is too big. Using %d", (int)wanted_score, (int)OOM_SCORE_ADJ_MAX);
        wanted_score = OOM_SCORE_ADJ_MAX;
    }

    if(old_score == wanted_score) {
        info("Out-Of-Memory (OOM) score is already set to the wanted value %d", (int)old_score);
        return;
    }

    int written = 0;
    int fd = open("/proc/self/oom_score_adj", O_WRONLY);
    if(fd != -1) {
        snprintfz(buf, 30, "%d", (int)wanted_score);
        ssize_t len = strlen(buf);
        if(len > 0 && write(fd, buf, (size_t)len) == len) written = 1;
        close(fd);

        if(written) {
            if(read_single_signed_number_file("/proc/self/oom_score_adj", &final_score))
                error("Adjusted my Out-Of-Memory (OOM) score to %d, but cannot verify it.", (int)wanted_score);
            else if(final_score == wanted_score)
                info("Adjusted my Out-Of-Memory (OOM) score from %d to %d.", (int)old_score, (int)final_score);
            else
                error("Adjusted my Out-Of-Memory (OOM) score from %d to %d, but it has been set to %d.", (int)old_score, (int)wanted_score, (int)final_score);
        }
        else
            error("Failed to adjust my Out-Of-Memory (OOM) score to %d. Running with %d. (systemd systems may change it via netdata.service)", (int)wanted_score, (int)old_score);
    }
    else
        error("Failed to adjust my Out-Of-Memory (OOM) score. Cannot open /proc/self/oom_score_adj for writing.");
}

static void process_nice_level(void) {
#ifdef HAVE_NICE
    int nice_level = (int)config_get_number(CONFIG_SECTION_GLOBAL, "process nice level", 19);
    if(nice(nice_level) == -1) error("Cannot set netdata CPU nice level to %d.", nice_level);
    else debug(D_SYSTEM, "Set netdata nice level to %d.", nice_level);
#endif // HAVE_NICE
};

#ifdef HAVE_SCHED_SETSCHEDULER

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

#ifdef SCHED_IDLE
        { "idle", SCHED_IDLE, 0, SCHED_FLAG_NONE },
#endif

#ifdef SCHED_OTHER
        { "nice",  SCHED_OTHER, 0, SCHED_FLAG_USE_NICE },
        { "other", SCHED_OTHER, 0, SCHED_FLAG_USE_NICE },
#endif

#ifdef SCHED_RR
        { "rr", SCHED_RR, 0, SCHED_FLAG_PRIORITY_CONFIGURABLE },
#endif

#ifdef SCHED_FIFO
        { "fifo", SCHED_FIFO, 0, SCHED_FLAG_PRIORITY_CONFIGURABLE },
#endif

#ifdef SCHED_BATCH
        { "batch", SCHED_BATCH, 0, SCHED_FLAG_USE_NICE },
#endif

        // do not change the scheduling priority
        { "keep", 0, 0, SCHED_FLAG_KEEP_AS_IS },
        { "none", 0, 0, SCHED_FLAG_KEEP_AS_IS },

        // array termination
        { NULL, 0, 0, 0 }
};

static void sched_setscheduler_set(void) {

    if(scheduler_defaults[0].name) {
        const char *name = scheduler_defaults[0].name;
        int policy = scheduler_defaults[0].policy, priority = scheduler_defaults[0].priority;
        uint8_t flags = scheduler_defaults[0].flags;
        int found = 0;

        // read the configuration
        name = config_get(CONFIG_SECTION_GLOBAL, "process scheduling policy", name);
        int i;
        for(i = 0 ; scheduler_defaults[i].name ; i++) {
            if(!strcmp(name, scheduler_defaults[i].name)) {
                found = 1;
                policy = scheduler_defaults[i].policy;
                priority = scheduler_defaults[i].priority;
                flags = scheduler_defaults[i].flags;

                if(flags & SCHED_FLAG_KEEP_AS_IS)
                    return;

                if(flags & SCHED_FLAG_PRIORITY_CONFIGURABLE)
                    priority = (int)config_get_number(CONFIG_SECTION_GLOBAL, "process scheduling priority", priority);

#ifdef HAVE_SCHED_GET_PRIORITY_MIN
                errno = 0;
                if(priority < sched_get_priority_min(policy)) {
                    error("scheduler %s (%d) priority %d is below the minimum %d. Using the minimum.", name, policy, priority, sched_get_priority_min(policy));
                    priority = sched_get_priority_min(policy);
                }
#endif
#ifdef HAVE_SCHED_GET_PRIORITY_MAX
                errno = 0;
                if(priority > sched_get_priority_max(policy)) {
                    error("scheduler %s (%d) priority %d is above the maximum %d. Using the maximum.", name, policy, priority, sched_get_priority_max(policy));
                    priority = sched_get_priority_max(policy);
                }
#endif
                break;
            }
        }

        if(!found) {
            error("Unknown scheduling policy '%s' - falling back to nice", name);
            goto fallback;
        }

        const struct sched_param param = {
                .sched_priority = priority
        };

        errno = 0;
        i = sched_setscheduler(0, policy, &param);
        if(i != 0) {
            error("Cannot adjust netdata scheduling policy to %s (%d), with priority %d. Falling back to nice.", name, policy, priority);
        }
        else {
            info("Adjusted netdata scheduling policy to %s (%d), with priority %d.", name, policy, priority);
            if(!(flags & SCHED_FLAG_USE_NICE))
                return;
        }
    }

fallback:
    process_nice_level();
}
#else
static void sched_setscheduler_set(void) {
    process_nice_level();
}
#endif

int become_daemon(int dont_fork, const char *user)
{
    if(!dont_fork) {
        int i = fork();
        if(i == -1) {
            perror("cannot fork");
            exit(1);
        }
        if(i != 0) {
            exit(0); // the parent
        }

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
        if(i != 0) {
            exit(0); // the parent
        }
    }

    // generate our pid file
    int pidfd = -1;
    if(pidfile[0]) {
        pidfd = open(pidfile, O_WRONLY | O_CREAT, 0644);
        if(pidfd >= 0) {
            if(ftruncate(pidfd, 0) != 0)
                error("Cannot truncate pidfile '%s'.", pidfile);

            char b[100];
            sprintf(b, "%d\n", getpid());
            ssize_t i = write(pidfd, b, strlen(b));
            if(i <= 0)
                error("Cannot write pidfile '%s'.", pidfile);
        }
        else error("Failed to open pidfile '%s'.", pidfile);
    }

    // Set new file permissions
    umask(0007);

    // adjust my Out-Of-Memory score
    oom_score_adj();

    // never become a problem
    sched_setscheduler_set();

    if(user && *user) {
        if(become_user(user, pidfd) != 0) {
            error("Cannot become user '%s'. Continuing as we are.", user);
        }
        else debug(D_SYSTEM, "Successfully became user '%s'.", user);
    }
    else {
        create_needed_dir(netdata_configured_cache_dir, getuid(), getgid());
        create_needed_dir(netdata_configured_varlib_dir, getuid(), getgid());
    }

    if(pidfd != -1)
        close(pidfd);

    return(0);
}
