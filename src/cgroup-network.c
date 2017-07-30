#include "common.h"

#ifdef HAVE_SETNS
#ifndef _GNU_SOURCE
#define _GNU_SOURCE             /* See feature_test_macros(7) */
#endif
#include <sched.h>
#endif

// ----------------------------------------------------------------------------
// callback required by fatal()

void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

struct iface {
    const char *device;
    uint32_t hash;

    unsigned int ifindex;
    unsigned int iflink;

    struct iface *next;
};

unsigned int read_iface_iflink(const char *prefix, const char *iface) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/sys/class/net/%s/iflink", prefix?prefix:"", iface);

    unsigned long long iflink = 0;
    int ret = read_single_number_file(filename, &iflink);
    if(ret) error("Cannot read '%s'.", filename);

    return (unsigned int)iflink;
}

unsigned int read_iface_ifindex(const char *prefix, const char *iface) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/sys/class/net/%s/ifindex", prefix?prefix:"", iface);

    unsigned long long ifindex = 0;
    int ret = read_single_number_file(filename, &ifindex);
    if(ret) error("Cannot read '%s'.", filename);

    return (unsigned int)ifindex;
}

struct iface *read_proc_net_dev(const char *prefix) {
    procfile *ff = NULL;
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s%s", prefix?prefix:"", "/proc/net/dev");
    ff = procfile_open(filename, " \t,:|", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) {
        error("Cannot open file '%s'", filename);
        return NULL;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) {
        error("Cannot read file '%s'", filename);
        return NULL;
    }

    size_t lines = procfile_lines(ff), l;
    struct iface *root = NULL;
    for(l = 2; l < lines ;l++) {
        if (unlikely(procfile_linewords(ff, l) < 1)) continue;

        struct iface *t = callocz(1, sizeof(struct iface));
        t->device = strdupz(procfile_lineword(ff, l, 0));
        t->hash = simple_hash(t->device);
        t->ifindex = read_iface_ifindex(prefix, t->device);
        t->iflink  = read_iface_iflink(prefix, t->device);
        t->next = root;
        root = t;
    }

    return root;
}

int iface_is_eligible(struct iface *iface) {
    if(iface->iflink != iface->ifindex)
        return 1;

    return 0;
}

int eligible_ifaces(struct iface *root) {
    int eligible = 0;

    struct iface *t;
    for(t = root; t ; t = t->next)
        if(iface_is_eligible(t))
            eligible++;

    return eligible;
}

static void continue_as_child(void) {
    pid_t child = fork();
    int status;
    pid_t ret;

    if (child < 0)
        error("fork() failed");

    /* Only the child returns */
    if (child == 0)
        return;

    for (;;) {
        ret = waitpid(child, &status, WUNTRACED);
        if ((ret == child) && (WIFSTOPPED(status))) {
            /* The child suspended so suspend us as well */
            kill(getpid(), SIGSTOP);
            kill(child, SIGCONT);
        } else {
            break;
        }
    }

    /* Return the child's exit code if possible */
    if (WIFEXITED(status)) {
        exit(WEXITSTATUS(status));
    } else if (WIFSIGNALED(status)) {
        kill(getpid(), WTERMSIG(status));
    }

    exit(EXIT_FAILURE);
}

int proc_pid_fd(const char *prefix, const char *ns, pid_t pid) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/%d/%s", prefix?prefix:"", (int)pid, ns);
    int fd = open(filename, O_RDONLY);

    if(fd == -1)
        error("Cannot open file '%s'", filename);

    return fd;
}

static struct ns {
    int nstype;
    int fd;
    int status;
    const char *name;
    const char *path;
} all_ns[] = {
        // { .nstype = CLONE_NEWUSER,   .fd = -1, .status = -1, .name = "user",    .path = "ns/user"   },
        // { .nstype = CLONE_NEWCGROUP, .fd = -1, .status = -1, .name = "cgroup",  .path = "ns/cgroup" },
        // { .nstype = CLONE_NEWIPC,    .fd = -1, .status = -1, .name = "ipc",     .path = "ns/ipc"    },
        // { .nstype = CLONE_NEWUTS,    .fd = -1, .status = -1, .name = "uts",     .path = "ns/uts"    },
        { .nstype = CLONE_NEWNET,    .fd = -1, .status = -1, .name = "network", .path = "ns/net"    },
        { .nstype = CLONE_NEWPID,    .fd = -1, .status = -1, .name = "pid",     .path = "ns/pid"    },
        { .nstype = CLONE_NEWNS,     .fd = -1, .status = -1, .name = "mount",   .path = "ns/mnt"    },

        // terminator
        { .nstype = 0,               .fd = -1, .status = -1, .name = NULL,      .path = NULL        }
};

int switch_namespace(const char *prefix, pid_t pid) {
#ifdef HAVE_SETNS

    int i;
    for(i = 0; all_ns[i].name ; i++)
        all_ns[i].fd = proc_pid_fd(prefix, all_ns[i].path, pid);

    int root_fd = proc_pid_fd(prefix, "root", pid);
    int cwd_fd  = proc_pid_fd(prefix, "cwd", pid);

    setgroups(0, NULL);

    // 2 passes - found it at nsenter source code
    // this is related CLONE_NEWUSER functionality

    int pass, errors = 0;
    for(pass = 0; pass < 2 ;pass++) {
        for(i = 0; all_ns[i].name ; i++) {
            if (all_ns[i].fd != -1 && all_ns[i].status == -1) {
                if(setns(all_ns[i].fd, all_ns[i].nstype) == -1) {
                    if(pass == 1) {
                        all_ns[i].status = 0;
                        error("Cannot switch to %s namespace of pid %d", all_ns[i].name, (int) pid);
                        errors++;
                    }
                }
                else
                    all_ns[i].status = 1;
            }
        }
    }

    setgroups(0, NULL);

    if(root_fd != -1) {
        if(fchdir(root_fd) < 0)
            error("Cannot fchdir() to pid %d root directory", (int)pid);

        if(chroot(".") < 0)
            error("Cannot chroot() to pid %d root directory", (int)pid);

        close(root_fd);
    }

    if(cwd_fd != -1) {
        if(fchdir(cwd_fd) < 0)
            error("Cannot fchdir() to pid %d current working directory", (int)pid);

        close(cwd_fd);
    }

    int do_fork = 0;
    for(i = 0; all_ns[i].name ; i++)
        if(all_ns[i].fd != -1) {

            // CLONE_NEWPID requires a fork() to become effective
            if(all_ns[i].nstype == CLONE_NEWPID && all_ns[i].status)
                do_fork = 1;

            close(all_ns[i].fd);
        }

    if(do_fork)
        continue_as_child();

    return 0;

#else

    errno = ENOSYS;
    error("setns() is missing on this system.");
    return 1;

#endif
}

void usage(void) {
    fprintf(stderr, "%s -p PID\n", program_name);
    exit(1);
}

int main(int argc, char **argv) {
    pid_t pid = 0;

    program_name = argv[0];
    program_version = VERSION;
    error_log_syslog = 0;

    if(argc == 2 && (!strcmp(argv[1], "version") || !strcmp(argv[1], "-version") || !strcmp(argv[1], "--version") || !strcmp(argv[1], "-v") || !strcmp(argv[1], "-V"))) {
        fprintf(stderr, "cgroup-network %s\n", VERSION);
        exit(0);
    }

    if(argc != 3 || strcmp(argv[1], "-p") != 0)
        usage();

    pid = atoi(argv[2]);
    if(pid <= 0)
        fatal("Invalid pid %d", (int)pid);

    struct iface *host, *cgroup, *h, *c;
    const char *prefix = getenv("NETDATA_HOST_PREFIX");

    host = read_proc_net_dev(prefix);
    if(!host)
        fatal("cannot read host interface list.");

    if(!eligible_ifaces(host))
        fatal("there are no double-linked host interfaces available.");

    if(switch_namespace(prefix, pid))
        fatal("cannot switch to the namespace of pid %u", (unsigned int)pid);

    cgroup = read_proc_net_dev(NULL);
    if(!cgroup)
        fatal("cannot read cgroup interface list.");

    if(!eligible_ifaces(cgroup))
        fatal("there are not double-linked cgroup interfaces available.");

    int found = 0;
    for(h = host; h ; h = h->next) {
        if(iface_is_eligible(h)) {
            for (c = cgroup; c; c = c->next) {
                if(iface_is_eligible(c) && h->ifindex == c->iflink && h->iflink == c->ifindex) {
                    printf("%s %s\n", h->device, c->device);
                    found++;
                }
            }
        }
    }

    if(!found)
        return 1;

    return 0;
}
