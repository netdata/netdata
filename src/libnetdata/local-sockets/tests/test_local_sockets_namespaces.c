// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

static int fixture_open(const char *path, int flags, ...);
static int fixture_fstat(int fd, struct stat *statbuf);
static int fixture_close(int fd);
static ssize_t fixture_read(int fd, void *buf, size_t count);
static int fixture_setns(int fd, int nstype);
static SPAWN_INSTANCE *fixture_spawn_server_exec(
    SPAWN_SERVER *server,
    int stderr_fd,
    int custom_fd,
    const char **argv,
    const void *data,
    size_t data_size,
    SPAWN_INSTANCE_TYPE type);
static int fixture_spawn_server_instance_read_fd(SPAWN_INSTANCE *si);
static int fixture_spawn_server_exec_kill(SPAWN_SERVER *server, SPAWN_INSTANCE *si, int timeout_ms);

#define open fixture_open
#define fstat fixture_fstat
#define close fixture_close
#define read fixture_read
#define setns fixture_setns
#define spawn_server_exec fixture_spawn_server_exec
#define spawn_server_instance_read_fd fixture_spawn_server_instance_read_fd
#define spawn_server_exec_kill fixture_spawn_server_exec_kill
#include "libnetdata/local-sockets/local-sockets.h"
#undef open
#undef fstat
#undef close
#undef read
#undef setns
#undef spawn_server_exec
#undef spawn_server_instance_read_fd
#undef spawn_server_exec_kill

#define FIXTURE_MAX_RULES 32
#define FIXTURE_MAX_FDS 64

enum candidate_behavior {
    CANDIDATE_VERIFIED,
    CANDIDATE_OPEN_FAILED,
    CANDIDATE_FSTAT_FAILED,
    CANDIDATE_MOVED,
};

enum attempt_behavior {
    ATTEMPT_SPAWN_FAILED,
    ATTEMPT_SETNS_EPERM,
    ATTEMPT_PROTOCOL_FAILED,
    ATTEMPT_EMPTY_SUCCESS,
    ATTEMPT_DATA_SUCCESS,
};

struct candidate_rule {
    pid_t pid;
    uint64_t namespace_inode;
    enum candidate_behavior behavior;
};

struct attempt_rule {
    uint64_t namespace_inode;
    enum attempt_behavior behavior;
    size_t calls;
};

struct fixture_fd {
    int fd;
    pid_t pid;
    bool open;
};

struct namespace_fixture {
    struct candidate_rule candidates[FIXTURE_MAX_RULES];
    size_t candidates_count;
    struct attempt_rule attempts[FIXTURE_MAX_RULES];
    size_t attempts_count;
    struct fixture_fd fds[FIXTURE_MAX_FDS];
    size_t fds_count;

    uint8_t read_data[(sizeof(LOCAL_SOCKET) + sizeof(size_t)) * 2];
    size_t read_size;
    size_t read_offset;
    int read_errno;

    size_t open_calls;
    size_t fstat_calls;
    size_t close_calls;
    size_t spawn_calls;
    size_t kill_calls;
    bool spawn_received_open_fd;
};

static struct namespace_fixture fixture;
static char fake_spawn_server;
static char fake_spawn_instance;

#define FIXTURE_REQUIRE(condition) do {                                                        \
    if(!(condition)) {                                                                         \
        fprintf(stderr, "FAILED: fixture invariant '%s' at %s:%d\n", #condition, __FILE__, __LINE__); \
        abort();                                                                               \
    }                                                                                          \
} while(0)

static bool expect_true(bool condition, const char *message) {
    if(condition)
        return true;

    fprintf(stderr, "FAILED: %s\n", message);
    return false;
}

static void fixture_reset(void) {
    memset(&fixture, 0, sizeof(fixture));
}

static struct candidate_rule *candidate_rule_for_pid(pid_t pid) {
    for(size_t i = 0; i < fixture.candidates_count; i++) {
        if(fixture.candidates[i].pid == pid)
            return &fixture.candidates[i];
    }

    return NULL;
}

static void set_candidate_rule(pid_t pid, uint64_t namespace_inode, enum candidate_behavior behavior) {
    struct candidate_rule *rule = candidate_rule_for_pid(pid);
    if(!rule) {
        FIXTURE_REQUIRE(fixture.candidates_count < FIXTURE_MAX_RULES);
        rule = &fixture.candidates[fixture.candidates_count++];
    }

    rule->pid = pid;
    rule->namespace_inode = namespace_inode;
    rule->behavior = behavior;
}

static struct attempt_rule *attempt_rule_for_namespace(uint64_t namespace_inode) {
    for(size_t i = 0; i < fixture.attempts_count; i++) {
        if(fixture.attempts[i].namespace_inode == namespace_inode)
            return &fixture.attempts[i];
    }

    return NULL;
}

static void set_attempt_rule(uint64_t namespace_inode, enum attempt_behavior behavior) {
    struct attempt_rule *rule = attempt_rule_for_namespace(namespace_inode);
    if(!rule) {
        FIXTURE_REQUIRE(fixture.attempts_count < FIXTURE_MAX_RULES);
        rule = &fixture.attempts[fixture.attempts_count++];
    }

    rule->namespace_inode = namespace_inode;
    rule->behavior = behavior;
    rule->calls = 0;
}

static pid_t pid_from_namespace_path(const char *path) {
    const char *proc = strstr(path, "/proc/");
    if(!proc)
        return 0;

    char *end = NULL;
    long value = strtol(proc + strlen("/proc/"), &end, 10);
    if(value <= 0 || !end || strcmp(end, "/ns/net") != 0)
        return 0;

    return (pid_t)value;
}

static struct fixture_fd *fixture_fd_for_number(int fd) {
    for(size_t i = 0; i < fixture.fds_count; i++) {
        if(fixture.fds[i].fd == fd)
            return &fixture.fds[i];
    }

    return NULL;
}

static int fixture_open(const char *path, int flags __maybe_unused, ...) {
    fixture.open_calls++;

    pid_t pid = pid_from_namespace_path(path);
    struct candidate_rule *rule = candidate_rule_for_pid(pid);
    if(!rule || rule->behavior == CANDIDATE_OPEN_FAILED) {
        errno = ENOENT;
        return -1;
    }

    FIXTURE_REQUIRE(fixture.fds_count < FIXTURE_MAX_FDS);
    struct fixture_fd *record = &fixture.fds[fixture.fds_count++];
    record->fd = 100 + (int)fixture.fds_count;
    record->pid = pid;
    record->open = true;
    return record->fd;
}

static int fixture_fstat(int fd, struct stat *statbuf) {
    fixture.fstat_calls++;

    struct fixture_fd *record = fixture_fd_for_number(fd);
    struct candidate_rule *rule = record ? candidate_rule_for_pid(record->pid) : NULL;
    FIXTURE_REQUIRE(record && record->open && rule);

    if(rule->behavior == CANDIDATE_FSTAT_FAILED) {
        errno = ENOENT;
        return -1;
    }

    memset(statbuf, 0, sizeof(*statbuf));
    statbuf->st_ino = (ino_t)(rule->namespace_inode + (rule->behavior == CANDIDATE_MOVED ? 1 : 0));
    return 0;
}

static int fixture_close(int fd) {
    struct fixture_fd *record = fixture_fd_for_number(fd);
    FIXTURE_REQUIRE(record && record->open);
    record->open = false;
    fixture.close_calls++;
    return 0;
}

static void append_read_data(const void *data, size_t size) {
    FIXTURE_REQUIRE(fixture.read_size + size <= sizeof(fixture.read_data));
    memcpy(&fixture.read_data[fixture.read_size], data, size);
    fixture.read_size += size;
}

static void prepare_attempt_stream(enum attempt_behavior behavior) {
    fixture.read_size = 0;
    fixture.read_offset = 0;
    fixture.read_errno = 0;

    if(behavior == ATTEMPT_PROTOCOL_FAILED) {
        fixture.read_errno = EIO;
        return;
    }

    if(behavior == ATTEMPT_EMPTY_SUCCESS || behavior == ATTEMPT_DATA_SUCCESS) {
        size_t no_cmdline = 0;

        if(behavior == ATTEMPT_DATA_SUCCESS) {
            LOCAL_SOCKET socket = {
                .inode = 90001,
                .state = TCP_ESTABLISHED,
                .local = {
                    .protocol = IPPROTO_TCP,
                    .family = AF_INET,
                    .port = 12345,
                },
                .remote = {
                    .protocol = IPPROTO_TCP,
                    .family = AF_INET,
                    .port = 443,
                },
                .uid = UID_UNSET,
            };
            append_read_data(&socket, sizeof(socket));
            append_read_data(&no_cmdline, sizeof(no_cmdline));
        }

        static const LOCAL_SOCKET terminator = LOCAL_SOCKET_TERMINATOR;
        append_read_data(&terminator, sizeof(terminator));
        append_read_data(&no_cmdline, sizeof(no_cmdline));
    }
}

static ssize_t fixture_read(int fd __maybe_unused, void *buf, size_t count) {
    if(fixture.read_errno) {
        errno = fixture.read_errno;
        fixture.read_errno = 0;
        return -1;
    }

    if(fixture.read_offset == fixture.read_size)
        return 0;

    size_t remaining = fixture.read_size - fixture.read_offset;
    if(count > remaining)
        count = remaining;
    memcpy(buf, &fixture.read_data[fixture.read_offset], count);
    fixture.read_offset += count;
    return (ssize_t)count;
}

static int fixture_setns(int fd __maybe_unused, int nstype __maybe_unused) {
    errno = EPERM;
    return -1;
}

static void run_setns_eperm_callback(const void *data, size_t data_size, int custom_fd) {
    pid_t child = fork();
    FIXTURE_REQUIRE(child >= 0);
    if(child == 0) {
        SPAWN_REQUEST request = {
            .fds = { STDIN_FILENO, STDOUT_FILENO, STDERR_FILENO, custom_fd },
            .data = data,
            .data_size = data_size,
            .type = SPAWN_INSTANCE_TYPE_CALLBACK,
        };
        _exit(local_sockets_spawn_server_callback(&request));
    }

    int status = 0;
    pid_t waited;
    do {
        waited = waitpid(child, &status, 0);
    } while(waited == -1 && errno == EINTR);
    FIXTURE_REQUIRE(waited == child);
    FIXTURE_REQUIRE(WIFEXITED(status));
    FIXTURE_REQUIRE(WEXITSTATUS(status) == EXIT_FAILURE);
}

static SPAWN_INSTANCE *fixture_spawn_server_exec(
    SPAWN_SERVER *server __maybe_unused,
    int stderr_fd __maybe_unused,
    int custom_fd,
    const char **argv __maybe_unused,
    const void *data,
    size_t data_size,
    SPAWN_INSTANCE_TYPE type) {
    fixture.spawn_calls++;
    FIXTURE_REQUIRE(type == SPAWN_INSTANCE_TYPE_CALLBACK);
    FIXTURE_REQUIRE(data_size == sizeof(struct local_sockets_ns_req));

    struct fixture_fd *record = fixture_fd_for_number(custom_fd);
    FIXTURE_REQUIRE(record && record->open);
    fixture.spawn_received_open_fd = true;

    const struct local_sockets_ns_req *request = data;
    struct attempt_rule *rule = attempt_rule_for_namespace(request->ns_state.net_ns_inode);
    FIXTURE_REQUIRE(rule);
    rule->calls++;

    if(rule->behavior == ATTEMPT_SPAWN_FAILED)
        return NULL;

    if(rule->behavior == ATTEMPT_SETNS_EPERM)
        run_setns_eperm_callback(data, data_size, custom_fd);

    prepare_attempt_stream(rule->behavior);
    return (SPAWN_INSTANCE *)&fake_spawn_instance;
}

static int fixture_spawn_server_instance_read_fd(SPAWN_INSTANCE *si) {
    FIXTURE_REQUIRE(si == (SPAWN_INSTANCE *)&fake_spawn_instance);
    return 200;
}

static int fixture_spawn_server_exec_kill(
    SPAWN_SERVER *server __maybe_unused,
    SPAWN_INSTANCE *si,
    int timeout_ms __maybe_unused) {
    FIXTURE_REQUIRE(si == (SPAWN_INSTANCE *)&fake_spawn_instance);
    fixture.kill_calls++;
    return 0;
}

static void init_state(LS_STATE *ls) {
    memset(ls, 0, sizeof(*ls));
    ls->config.namespaces = true;
    ls->config.report = true;
    ls->config.max_errors = 0;
    ls->config.max_concurrent_namespaces = 1;
    ls->spawn_server = (SPAWN_SERVER *)&fake_spawn_server;
    local_sockets_init(ls);
    ls->proc_self_net_ns_inode = UINT64_MAX;
}

static void add_pid_socket(LS_STATE *ls, uint64_t socket_inode, pid_t pid, uint64_t namespace_inode) {
    struct pid_socket *ps = aral_callocz(ls->pid_socket_aral);
    ps->inode = socket_inode;
    ps->pid = pid;
    ps->uid = UID_UNSET;
    ps->net_ns_inode = namespace_inode;

    XXH64_hash_t hash = XXH3_64bits(&socket_inode, sizeof(socket_inode));
    SIMPLE_HASHTABLE_SLOT_PID_SOCKET *slot =
        simple_hashtable_get_slot_PID_SOCKET(&ls->pid_sockets_hashtable, hash, &socket_inode, true);
    FIXTURE_REQUIRE(!SIMPLE_HASHTABLE_SLOT_DATA(slot));
    simple_hashtable_set_slot_PID_SOCKET(&ls->pid_sockets_hashtable, slot, hash, ps);
}

static void add_namespace(LS_STATE *ls, uint64_t namespace_inode) {
    XXH64_hash_t hash = XXH3_64bits(&namespace_inode, sizeof(namespace_inode));
    SIMPLE_HASHTABLE_SLOT_NET_NS *slot =
        simple_hashtable_get_slot_NET_NS(&ls->ns_hashtable, hash, &namespace_inode, true);
    simple_hashtable_set_slot_NET_NS(&ls->ns_hashtable, slot, hash, namespace_inode);
}

static struct pid_socket *first_pid_socket_for_namespace(LS_STATE *ls, uint64_t namespace_inode) {
    for(SIMPLE_HASHTABLE_SLOT_PID_SOCKET *slot =
            simple_hashtable_first_read_only_PID_SOCKET(&ls->pid_sockets_hashtable);
        slot;
        slot = simple_hashtable_next_read_only_PID_SOCKET(&ls->pid_sockets_hashtable, slot)) {
        struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(slot);
        if(ps && ps->net_ns_inode == namespace_inode)
            return ps;
    }

    return NULL;
}

static bool all_namespace_fds_closed(void) {
    for(size_t i = 0; i < fixture.fds_count; i++) {
        if(fixture.fds[i].open)
            return false;
    }
    return true;
}

static void run_worker(LS_STATE *ls, uint64_t namespace_inode) {
    struct local_sockets_namespace_worker worker = {
        .ls = ls,
        .inode = namespace_inode,
    };
    local_sockets_get_namespace_sockets_worker(&worker);
}

struct discovery_process {
    pid_t pid;
    uint64_t namespace_inode;
    const uint64_t *socket_inodes;
    size_t socket_inodes_count;
};

static void fixture_make_directory(const char *path) {
    FIXTURE_REQUIRE(mkdir(path, 0700) == 0);
}

static void fixture_make_symlink(const char *target, const char *path) {
    FIXTURE_REQUIRE(symlink(target, path) == 0);
}

static void fixture_create_proc_process(const char *root, const struct discovery_process *process) {
    char pid_dir[FILENAME_MAX + 1];
    char fd_dir[FILENAME_MAX + 1];
    char ns_dir[FILENAME_MAX + 1];
    char path[FILENAME_MAX + 1];
    char target[128];

    snprintfz(pid_dir, sizeof(pid_dir), "%s/%d", root, process->pid);
    snprintfz(fd_dir, sizeof(fd_dir), "%s/fd", pid_dir);
    snprintfz(ns_dir, sizeof(ns_dir), "%s/ns", pid_dir);
    fixture_make_directory(pid_dir);
    fixture_make_directory(fd_dir);
    fixture_make_directory(ns_dir);

    snprintfz(path, sizeof(path), "%s/net", ns_dir);
    snprintfz(target, sizeof(target), "net:[%" PRIu64 "]", process->namespace_inode);
    fixture_make_symlink(target, path);

    for(size_t i = 0; i < process->socket_inodes_count; i++) {
        snprintfz(path, sizeof(path), "%s/%zu", fd_dir, i + 3);
        snprintfz(target, sizeof(target), "socket:[%" PRIu64 "]", process->socket_inodes[i]);
        fixture_make_symlink(target, path);
    }
}

static void fixture_remove_proc_process(const char *root, const struct discovery_process *process) {
    char pid_dir[FILENAME_MAX + 1];
    char fd_dir[FILENAME_MAX + 1];
    char ns_dir[FILENAME_MAX + 1];
    char path[FILENAME_MAX + 1];

    snprintfz(pid_dir, sizeof(pid_dir), "%s/%d", root, process->pid);
    snprintfz(fd_dir, sizeof(fd_dir), "%s/fd", pid_dir);
    snprintfz(ns_dir, sizeof(ns_dir), "%s/ns", pid_dir);

    for(size_t i = 0; i < process->socket_inodes_count; i++) {
        snprintfz(path, sizeof(path), "%s/%zu", fd_dir, i + 3);
        FIXTURE_REQUIRE(unlink(path) == 0);
    }

    snprintfz(path, sizeof(path), "%s/net", ns_dir);
    FIXTURE_REQUIRE(unlink(path) == 0);
    FIXTURE_REQUIRE(rmdir(fd_dir) == 0);
    FIXTURE_REQUIRE(rmdir(ns_dir) == 0);
    FIXTURE_REQUIRE(rmdir(pid_dir) == 0);
}

static bool test_proc_discovery_deduplicates_namespace_attempts(void) {
    fixture_reset();
    LS_STATE ls;
    init_state(&ls);
    ls.config.uid = false;
    ls.config.pid = false;
    ls.config.comm = false;
    ls.config.cmdline = false;

    char root[] = "/tmp/netdata-local-sockets-namespaces.XXXXXX";
    FIXTURE_REQUIRE(mkdtemp(root));

    static const uint64_t process_a_sockets[] = { 7001, 7002 };
    static const uint64_t process_b_sockets[] = { 7003, 7004, 7005 };
    static const uint64_t process_c_sockets[] = { 7006 };
    static const struct discovery_process processes[] = {
        { 91, 1501, process_a_sockets, _countof(process_a_sockets) },
        { 92, 1501, process_b_sockets, _countof(process_b_sockets) },
        { 93, 1502, process_c_sockets, _countof(process_c_sockets) },
    };

    for(size_t i = 0; i < _countof(processes); i++) {
        fixture_create_proc_process(root, &processes[i]);
        set_candidate_rule(processes[i].pid, processes[i].namespace_inode, CANDIDATE_VERIFIED);
    }
    set_attempt_rule(1501, ATTEMPT_SPAWN_FAILED);
    set_attempt_rule(1502, ATTEMPT_SPAWN_FAILED);

    bool ok = true;
    ok = expect_true(local_sockets_find_all_sockets_in_proc(&ls, root), "synthetic proc discovery failed") && ok;
    ok = expect_true(ls.pid_sockets_hashtable.used == 6, "synthetic proc discovery lost socket records") && ok;
    ok = expect_true(ls.ns_hashtable.used == 2, "synthetic proc discovery duplicated a namespace") && ok;

    local_sockets_namespaces(&ls);

    struct attempt_rule *attempt_a = attempt_rule_for_namespace(1501);
    struct attempt_rule *attempt_b = attempt_rule_for_namespace(1502);
    FIXTURE_REQUIRE(attempt_a && attempt_b);
    ok = expect_true(attempt_a->calls == 1, "same-namespace processes caused duplicate collection attempts") && ok;
    ok = expect_true(attempt_b->calls == 1, "second namespace had the wrong collection attempt count") && ok;
    ok = expect_true(fixture.open_calls == 2 && fixture.spawn_calls == 2, "discovered namespaces caused extra helper attempts") && ok;
    ok = expect_true(
        ls.stats.namespaces_found == 2 &&
            ls.stats.namespaces_forks_attempted == 2 &&
            ls.stats.namespaces_forks_failed == 2,
        "discovered namespace counters changed") && ok;
    ok = expect_true(fixture.close_calls == 2 && all_namespace_fds_closed(), "discovered namespace FDs leaked") && ok;

    local_sockets_cleanup(&ls);
    for(size_t i = 0; i < _countof(processes); i++)
        fixture_remove_proc_process(root, &processes[i]);
    FIXTURE_REQUIRE(rmdir(root) == 0);
    return ok;
}

static bool test_duplicate_pid_records_stop_after_spawn_failure(void) {
    fixture_reset();
    LS_STATE ls;
    init_state(&ls);

    const uint64_t namespace_inode = 1001;
    const pid_t pid = 41;
    for(uint64_t i = 0; i < 8; i++)
        add_pid_socket(&ls, 2000 + i, pid, namespace_inode);
    set_candidate_rule(pid, namespace_inode, CANDIDATE_VERIFIED);
    set_attempt_rule(namespace_inode, ATTEMPT_SPAWN_FAILED);

    run_worker(&ls, namespace_inode);

    bool ok = true;
    ok = expect_true(fixture.open_calls == 1, "duplicate PID/socket records caused repeated candidate opens") && ok;
    ok = expect_true(fixture.spawn_calls == 1, "spawn failure caused repeated namespace attempts") && ok;
    ok = expect_true(fixture.close_calls == 1 && all_namespace_fds_closed(), "spawn-failure namespace FD was not closed once") && ok;
    ok = expect_true(ls.stats.namespaces_forks_attempted == 1, "spawn-attempt counter changed") && ok;
    ok = expect_true(ls.stats.namespaces_forks_failed == 1, "spawn-failure counter changed") && ok;

    local_sockets_cleanup(&ls);
    return ok;
}

static bool test_verified_attempt_failures_stop_candidate_scan(void) {
    static const struct {
        const char *name;
        enum attempt_behavior behavior;
        bool remove_spawn_server;
    } cases[] = {
        { "missing spawn server", ATTEMPT_SPAWN_FAILED, true },
        { "spawn failure", ATTEMPT_SPAWN_FAILED, false },
        { "setns EPERM", ATTEMPT_SETNS_EPERM, false },
        { "protocol failure", ATTEMPT_PROTOCOL_FAILED, false },
    };

    bool ok = true;
    for(size_t c = 0; c < _countof(cases); c++) {
        fixture_reset();
        LS_STATE ls;
        init_state(&ls);

        const uint64_t namespace_inode = 1100 + c;
        const pid_t pid = 51 + (pid_t)c;
        for(uint64_t i = 0; i < 4; i++)
            add_pid_socket(&ls, 3000 + c * 10 + i, pid, namespace_inode);
        set_candidate_rule(pid, namespace_inode, CANDIDATE_VERIFIED);
        set_attempt_rule(namespace_inode, cases[c].behavior);
        if(cases[c].remove_spawn_server)
            ls.spawn_server = NULL;

        run_worker(&ls, namespace_inode);

        char message[160];
        snprintfz(message, sizeof(message), "%s retried a verified namespace candidate", cases[c].name);
        ok = expect_true(fixture.open_calls == 1, message) && ok;
        snprintfz(message, sizeof(message), "%s did not close its namespace FD", cases[c].name);
        ok = expect_true(fixture.close_calls == 1 && all_namespace_fds_closed(), message) && ok;
        snprintfz(message, sizeof(message), "%s started the wrong number of collection attempts", cases[c].name);
        ok = expect_true(fixture.spawn_calls == (cases[c].remove_spawn_server ? 0 : 1), message) && ok;
        snprintfz(message, sizeof(message), "%s did not attempt with the verified namespace FD open", cases[c].name);
        ok = expect_true(fixture.spawn_received_open_fd == !cases[c].remove_spawn_server, message) && ok;

        size_t expected_attempts = cases[c].remove_spawn_server ? 0 : 1;
        size_t expected_failures =
            (cases[c].remove_spawn_server || cases[c].behavior == ATTEMPT_SPAWN_FAILED) ? 1 : 0;
        size_t expected_unresponsive =
            (cases[c].behavior == ATTEMPT_SETNS_EPERM || cases[c].behavior == ATTEMPT_PROTOCOL_FAILED) ? 1 : 0;
        size_t expected_kills = expected_unresponsive;
        snprintfz(message, sizeof(message), "%s changed namespace attempt/failure counters", cases[c].name);
        ok = expect_true(
            ls.stats.namespaces_forks_attempted == expected_attempts &&
                ls.stats.namespaces_forks_failed == expected_failures &&
                ls.stats.namespaces_forks_unresponsive == expected_unresponsive,
            message) && ok;
        snprintfz(message, sizeof(message), "%s changed spawn-instance cleanup", cases[c].name);
        ok = expect_true(fixture.kill_calls == expected_kills, message) && ok;

        local_sockets_cleanup(&ls);
    }

    return ok;
}

static bool test_unavailable_candidates_fall_back(void) {
    static const struct {
        const char *name;
        enum candidate_behavior behavior;
        size_t expected_closes_before_attempt;
    } cases[] = {
        { "open failure", CANDIDATE_OPEN_FAILED, 0 },
        { "fstat failure", CANDIDATE_FSTAT_FAILED, 1 },
        { "moved PID", CANDIDATE_MOVED, 1 },
    };

    bool ok = true;
    for(size_t c = 0; c < _countof(cases); c++) {
        fixture_reset();
        LS_STATE ls;
        init_state(&ls);

        const uint64_t namespace_inode = 1200 + c;
        const pid_t pids[] = { 61, 62, 63 };
        for(size_t i = 0; i < _countof(pids); i++) {
            add_pid_socket(&ls, 4000 + c * 10 + i, pids[i], namespace_inode);
            set_candidate_rule(pids[i], namespace_inode, CANDIDATE_VERIFIED);
        }

        struct pid_socket *first = first_pid_socket_for_namespace(&ls, namespace_inode);
        FIXTURE_REQUIRE(first);
        set_candidate_rule(first->pid, namespace_inode, cases[c].behavior);
        set_attempt_rule(namespace_inode, ATTEMPT_SPAWN_FAILED);

        run_worker(&ls, namespace_inode);

        char message[160];
        snprintfz(message, sizeof(message), "%s did not stop after the first verified fallback candidate", cases[c].name);
        ok = expect_true(fixture.open_calls == 2 && fixture.spawn_calls == 1, message) && ok;
        snprintfz(message, sizeof(message), "%s leaked or double-closed a namespace FD", cases[c].name);
        ok = expect_true(
            fixture.close_calls == cases[c].expected_closes_before_attempt + 1 && all_namespace_fds_closed(), message) && ok;
        snprintfz(message, sizeof(message), "%s did not pass an open verified namespace FD to the attempt", cases[c].name);
        ok = expect_true(fixture.spawn_received_open_fd, message) && ok;
        snprintfz(message, sizeof(message), "%s changed namespace attempt/failure counters", cases[c].name);
        ok = expect_true(
            ls.stats.namespaces_forks_attempted == 1 && ls.stats.namespaces_forks_failed == 1,
            message) && ok;

        if(cases[c].behavior == CANDIDATE_MOVED)
            ok = expect_true(ls.stats.namespaces_invalid == 1, "moved-PID counter changed") && ok;
        else
            ok = expect_true(ls.stats.namespaces_absent == 1, "unavailable-PID counter changed") && ok;

        local_sockets_cleanup(&ls);
    }

    return ok;
}

static bool test_separate_namespaces_and_subsequent_scans(void) {
    fixture_reset();
    LS_STATE ls;
    init_state(&ls);

    const uint64_t namespace_a = 1301;
    const uint64_t namespace_b = 1302;
    for(uint64_t i = 0; i < 4; i++)
        add_pid_socket(&ls, 5000 + i, 71, namespace_a);
    for(uint64_t i = 0; i < 3; i++)
        add_pid_socket(&ls, 5100 + i, 72, namespace_b);
    set_candidate_rule(71, namespace_a, CANDIDATE_VERIFIED);
    set_candidate_rule(72, namespace_b, CANDIDATE_VERIFIED);
    set_attempt_rule(namespace_a, ATTEMPT_SPAWN_FAILED);
    set_attempt_rule(namespace_b, ATTEMPT_SPAWN_FAILED);
    add_namespace(&ls, namespace_a);
    add_namespace(&ls, namespace_b);

    local_sockets_namespaces(&ls);
    local_sockets_namespaces(&ls);

    struct attempt_rule *attempt_a = attempt_rule_for_namespace(namespace_a);
    struct attempt_rule *attempt_b = attempt_rule_for_namespace(namespace_b);
    bool ok = true;
    ok = expect_true(attempt_a->calls == 2 && attempt_b->calls == 2, "namespace failure incorrectly persisted across scans") && ok;
    ok = expect_true(fixture.spawn_calls == 4, "separate namespaces were retried per socket record") && ok;
    ok = expect_true(ls.stats.namespaces_found == 4, "namespace discovery counter changed across scans") && ok;
    ok = expect_true(
        ls.stats.namespaces_forks_attempted == 4 && ls.stats.namespaces_forks_failed == 4,
        "multi-namespace attempt/failure counters changed") && ok;
    ok = expect_true(fixture.close_calls == 4 && all_namespace_fds_closed(), "multi-namespace scans leaked namespace FDs") && ok;

    local_sockets_cleanup(&ls);
    return ok;
}

static bool test_empty_and_data_success(void) {
    static const struct {
        const char *name;
        enum attempt_behavior behavior;
        size_t expected_sockets;
    } cases[] = {
        { "empty success", ATTEMPT_EMPTY_SUCCESS, 0 },
        { "data success", ATTEMPT_DATA_SUCCESS, 1 },
    };

    bool ok = true;
    for(size_t c = 0; c < _countof(cases); c++) {
        fixture_reset();
        LS_STATE ls;
        init_state(&ls);

        const uint64_t namespace_inode = 1400 + c;
        add_pid_socket(&ls, 6000 + c, 81 + (pid_t)c, namespace_inode);
        set_candidate_rule(81 + (pid_t)c, namespace_inode, CANDIDATE_VERIFIED);
        set_attempt_rule(namespace_inode, cases[c].behavior);

        run_worker(&ls, namespace_inode);

        char message[160];
        snprintfz(message, sizeof(message), "%s did not stop after one collection attempt", cases[c].name);
        ok = expect_true(fixture.spawn_calls == 1 && fixture.kill_calls == 1, message) && ok;
        snprintfz(message, sizeof(message), "%s produced the wrong socket count", cases[c].name);
        ok = expect_true(ls.sockets_hashtable.used == cases[c].expected_sockets, message) && ok;
        snprintfz(message, sizeof(message), "%s leaked its namespace FD", cases[c].name);
        ok = expect_true(fixture.close_calls == 1 && all_namespace_fds_closed(), message) && ok;
        ok = expect_true(fixture.spawn_received_open_fd, "successful attempt did not receive an open namespace FD") && ok;
        ok = expect_true(
            ls.stats.namespaces_forks_attempted == 1 &&
                ls.stats.namespaces_forks_failed == 0 &&
                ls.stats.namespaces_forks_unresponsive == 0,
            "successful response changed namespace attempt/failure counters") && ok;

        if(cases[c].behavior == ATTEMPT_DATA_SUCCESS) {
            uint64_t socket_inode = 90001;
            XXH64_hash_t hash = XXH3_64bits(&socket_inode, sizeof(socket_inode));
            SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *slot =
                simple_hashtable_get_slot_LOCAL_SOCKET(&ls.sockets_hashtable, hash, &socket_inode, false);
            LOCAL_SOCKET *socket = SIMPLE_HASHTABLE_SLOT_DATA(slot);
            ok = expect_true(socket && socket->net_ns_inode == namespace_inode, "successful socket lost namespace identity") && ok;
            ok = expect_true(ls.stats.namespaces_sockets_new == 1, "successful socket counter changed") && ok;
        }

        local_sockets_cleanup(&ls);
    }

    return ok;
}

int main(void) {
    bool ok = true;

    ok = test_proc_discovery_deduplicates_namespace_attempts() && ok;
    ok = test_duplicate_pid_records_stop_after_spawn_failure() && ok;
    ok = test_verified_attempt_failures_stop_candidate_scan() && ok;
    ok = test_unavailable_candidates_fall_back() && ok;
    ok = test_separate_namespaces_and_subsequent_scans() && ok;
    ok = test_empty_and_data_success() && ok;

    return ok ? 0 : 1;
}
