#include "common.h"
#include <sched.h>

char pidfile[FILENAME_MAX + 1] = "";

void sig_handler_exit(int signo)
{
    if(signo) {
        error_log_limit_unlimited();
        error("Received signal %d. Exiting...", signo);
        netdata_exit = 1;
    }
}

void sig_handler_logrotate(int signo)
{
    if(signo) {
        error_log_limit_unlimited();
        info("Received signal %d to re-open the log files", signo);
        reopen_all_log_files();
        error_log_limit_reset();
    }
}

void sig_handler_save(int signo)
{
    if(signo) {
        error_log_limit_unlimited();
        info("Received signal %d to save the database...", signo);
        rrdset_save_all();
        error_log_limit_reset();
    }
}

void sig_handler_reload_health(int signo)
{
    if(signo) {
        error_log_limit_unlimited();
        info("Received signal %d to reload health configuration...", signo);
        health_reload();
        error_log_limit_reset();
    }
}

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

int become_user(const char *username, int pid_fd)
{
    struct passwd *pw = getpwnam(username);
    if(!pw) {
        error("User %s is not present.", username);
        return -1;
    }

    uid_t uid = pw->pw_uid;
    gid_t gid = pw->pw_gid;

    create_needed_dir(CACHE_DIR, uid, gid);
    create_needed_dir(VARLIB_DIR, uid, gid);

    if(pidfile[0]) {
        if(chown(pidfile, uid, gid) == -1)
            error("Cannot chown '%s' to %u:%u", pidfile, (unsigned int)uid, (unsigned int)gid);
    }

    int ngroups = (int)sysconf(_SC_NGROUPS_MAX);
    gid_t *supplementary_groups = NULL;
    if(ngroups) {
        supplementary_groups = mallocz(sizeof(gid_t) * ngroups);
        if(getgrouplist(username, gid, supplementary_groups, &ngroups) == -1) {
            error("Cannot get supplementary groups of user '%s'.", username);
            freez(supplementary_groups);
            supplementary_groups = NULL;
            ngroups = 0;
        }
    }

    chown_open_file(STDOUT_FILENO, uid, gid);
    chown_open_file(STDERR_FILENO, uid, gid);
    chown_open_file(stdaccess_fd, uid, gid);
    chown_open_file(pid_fd, uid, gid);

    if(supplementary_groups && ngroups) {
        if(setgroups(ngroups, supplementary_groups) == -1)
            error("Cannot set supplementary groups for user '%s'", username);

        freez(supplementary_groups);
        ngroups = 0;
    }

    if(setresgid(gid, gid, gid) != 0) {
        error("Cannot switch to user's %s group (gid: %u).", username, gid);
        return -1;
    }

    if(setresuid(uid, uid, uid) != 0) {
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

void oom_score_adj(int score) {
    int done = 0;
    int fd = open("/proc/self/oom_score_adj", O_WRONLY);
    if(fd != -1) {
        char buf[10 + 1];
        ssize_t len = snprintfz(buf, 10, "%d", score);
        if(write(fd, buf, len) == len) done = 1;
        close(fd);
    }

    if(!done)
        error("Cannot adjust my Out-Of-Memory score to %d.", score);
    else
        debug(D_SYSTEM, "Adjusted my Out-Of-Memory score to %d.", score);
}

int sched_setscheduler_idle(void) {
#ifdef SCHED_IDLE
    const struct sched_param param = {
        .sched_priority = 0
    };

    int i = sched_setscheduler(0, SCHED_IDLE, &param);
    if(i != 0)
        error("Cannot adjust my scheduling priority to IDLE.");
    else
        debug(D_SYSTEM, "Adjusted my scheduling priority to IDLE.");

    return i;
#else
    return -1;
#endif
}

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
    oom_score_adj(1000);

    // never become a problem
    if(sched_setscheduler_idle() != 0) {
        if(nice(19) == -1) error("Cannot lower my CPU priority.");
        else debug(D_SYSTEM, "Set my nice value to 19.");
    }

    if(user && *user) {
        if(become_user(user, pidfd) != 0) {
            error("Cannot become user '%s'. Continuing as we are.", user);
        }
        else debug(D_SYSTEM, "Successfully became user '%s'.", user);
    }
    else {
        create_needed_dir(CACHE_DIR, getuid(), getgid());
        create_needed_dir(VARLIB_DIR, getuid(), getgid());
    }

    if(pidfd != -1)
        close(pidfd);

    return(0);
}
