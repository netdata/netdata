// SPDX-License-Identifier: GPL-3.0-or-later

// macOS socket collection backend for local-sockets.h.
// Included only when OS_MACOS is defined; never compiled standalone.

#ifndef NETDATA_LOCAL_SOCKETS_MACOS_H
#define NETDATA_LOCAL_SOCKETS_MACOS_H

#include <errno.h>
#include <libproc.h>
#include <netinet/in.h>
#include <sys/proc_info.h>
#include <sys/sysctl.h>

// --------------------------------------------------------------------------------------------------------------------
// Normalize Darwin TSI_S_* states to Linux-compatible TCP_* values.

static inline int local_sockets_macos_normalize_tcp_state(int state) {
    switch(state) {
        case TSI_S_ESTABLISHED:  return TCP_ESTABLISHED;
        case TSI_S_SYN_SENT:     return TCP_SYN_SENT;
        case TSI_S_SYN_RECEIVED: return TCP_SYN_RECV;
        case TSI_S_FIN_WAIT_1:   return TCP_FIN_WAIT1;
        case TSI_S_FIN_WAIT_2:   return TCP_FIN_WAIT2;
        case TSI_S_TIME_WAIT:    return TCP_TIME_WAIT;
        case TSI_S_CLOSED:       return TCP_CLOSE;
        case TSI_S__CLOSE_WAIT:  return TCP_CLOSE_WAIT;
        case TSI_S_LAST_ACK:     return TCP_LAST_ACK;
        case TSI_S_LISTEN:       return TCP_LISTEN;
        case TSI_S_CLOSING:      return TCP_CLOSING;
        default:                 return 0;
    }
}

// --------------------------------------------------------------------------------------------------------------------

static inline bool local_sockets_macos_process_race_errno(int err) {
    return err == ESRCH || err == ENOENT;
}

static inline bool local_sockets_macos_permission_errno(int err) {
    return err == EPERM || err == EACCES;
}

static inline void local_sockets_macos_log_incomplete_summary(LS_STATE *ls) {
    if(!ls || !(ls->stats.macos_pid_fds_permission_denied ||
                ls->stats.macos_socket_fds_permission_denied ||
                ls->stats.macos_pid_lists_maybe_truncated ||
                ls->stats.macos_pid_fd_lists_maybe_truncated))
        return;

    nd_log_limit_static_global_var(erl, 60, 0);
    int saved_errno = errno;
    errno = 0;
    if(ls->stats.macos_pid_fds_permission_denied || ls->stats.macos_socket_fds_permission_denied) {
        if(ls->stats.macos_pid_lists_maybe_truncated || ls->stats.macos_pid_fd_lists_maybe_truncated)
            nd_log_limit(
                &erl,
                NDLS_COLLECTORS,
                NDLP_WARNING,
                "LOCAL-SOCKETS: macOS socket collection skipped %zu process FD lists and %zu socket FDs due to "
                "permissions and observed %zu PID list scans and %zu process FD lists that may have been truncated "
                "while scanning %zu PIDs and %zu socket FDs; results may be incomplete. "
                "Run network-viewer.plugin with Netdata privileged plugin permissions and grant Full Disk Access when required.",
                ls->stats.macos_pid_fds_permission_denied,
                ls->stats.macos_socket_fds_permission_denied,
                ls->stats.macos_pid_lists_maybe_truncated,
                ls->stats.macos_pid_fd_lists_maybe_truncated,
                ls->stats.macos_pids_seen,
                ls->stats.macos_socket_fds_seen);
        else
            nd_log_limit(
                &erl,
                NDLS_COLLECTORS,
                NDLP_WARNING,
                "LOCAL-SOCKETS: macOS socket collection skipped %zu process FD lists and %zu socket FDs due to "
                "permissions while scanning %zu PIDs and %zu socket FDs; results may be incomplete. "
                "Run network-viewer.plugin with Netdata privileged plugin permissions and grant Full Disk Access when required.",
                ls->stats.macos_pid_fds_permission_denied,
                ls->stats.macos_socket_fds_permission_denied,
                ls->stats.macos_pids_seen,
                ls->stats.macos_socket_fds_seen);
    }
    else
        nd_log_limit(
            &erl,
            NDLS_COLLECTORS,
            NDLP_WARNING,
            "LOCAL-SOCKETS: macOS socket collection observed %zu PID list scans and %zu process FD lists that may "
            "have been truncated while scanning %zu PIDs and %zu socket FDs; results may be incomplete.",
            ls->stats.macos_pid_lists_maybe_truncated,
            ls->stats.macos_pid_fd_lists_maybe_truncated,
            ls->stats.macos_pids_seen,
            ls->stats.macos_socket_fds_seen);
    errno = saved_errno;
}

static inline bool local_sockets_macos_read_bsdinfo(pid_t pid, struct proc_bsdinfo *bsd) {
    if(!bsd)
        return false;

    memset(bsd, 0, sizeof(*bsd));
    int rc = proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, bsd, sizeof(*bsd));
    return rc == (int)sizeof(*bsd);
}

static inline void local_sockets_macos_set_comm(LOCAL_SOCKET *n, const struct proc_bsdinfo *bsd) {
    if(!n || !bsd)
        return;

    if(bsd->pbi_name[0])
        strncpyz(n->comm, bsd->pbi_name, sizeof(n->comm) - 1);
    else if(bsd->pbi_comm[0])
        strncpyz(n->comm, bsd->pbi_comm, sizeof(n->comm) - 1);
}

static inline bool local_sockets_macos_read_cmdline(pid_t pid, char *cmdline, size_t size) {
    if(!cmdline || size < 2)
        return false;

    cmdline[0] = '\0';

    int mib[3] = { CTL_KERN, KERN_PROCARGS2, pid };
    size_t args_size = 0;
    if(sysctl(mib, 3, NULL, &args_size, NULL, 0) == -1 || args_size <= sizeof(int))
        goto fallback_path;

    char *args = mallocz(args_size);
    size_t used_size = args_size;
    if(sysctl(mib, 3, args, &used_size, NULL, 0) == -1 || used_size <= sizeof(int)) {
        freez(args);
        goto fallback_path;
    }

    int argc = 0;
    memcpy(&argc, args, sizeof(argc));
    if(argc <= 0) {
        freez(args);
        goto fallback_path;
    }

    char *ptr = args + sizeof(argc);
    char *end = args + used_size;

    while(ptr < end && *ptr)
        ptr++;
    while(ptr < end && *ptr == '\0')
        ptr++;

    size_t written = 0;
    int copied_args = 0;
    bool in_arg = false;

    while(ptr < end && written + 1 < size && copied_args < argc) {
        if(*ptr == '\0') {
            if(in_arg) {
                cmdline[written++] = ' ';
                in_arg = false;
                copied_args++;
            }
        }
        else {
            cmdline[written++] = *ptr;
            in_arg = true;
        }
        ptr++;
    }

    while(written > 0 && cmdline[written - 1] == ' ')
        written--;

    cmdline[written] = '\0';
    freez(args);

    if(cmdline[0]) {
        local_sockets_fix_cmdline(cmdline);
        return true;
    }

fallback_path:
    if(proc_pidpath(pid, cmdline, (uint32_t)size) > 0) {
        cmdline[size - 1] = '\0';
        local_sockets_fix_cmdline(cmdline);
        return true;
    }

    cmdline[0] = '\0';
    return false;
}

static inline bool local_sockets_macos_protocol_enabled(
    LS_STATE *ls,
    uint16_t family,
    uint16_t protocol)
{
    if(family == AF_INET && protocol == IPPROTO_TCP)
        return ls->config.tcp4;
    if(family == AF_INET6 && protocol == IPPROTO_TCP)
        return ls->config.tcp6;
    if(family == AF_INET && protocol == IPPROTO_UDP)
        return ls->config.udp4;
    if(family == AF_INET6 && protocol == IPPROTO_UDP)
        return ls->config.udp6;

    return false;
}

static inline const struct in_sockinfo *local_sockets_macos_in_sockinfo(const struct socket_info *si) {
    if(!si)
        return NULL;

    if(si->soi_kind == SOCKINFO_TCP)
        return &si->soi_proto.pri_tcp.tcpsi_ini;
    if(si->soi_kind == SOCKINFO_IN)
        return &si->soi_proto.pri_in;

    return NULL;
}

static inline uint16_t local_sockets_macos_socket_protocol(const struct socket_info *si) {
    if(!si)
        return 0;

    if(si->soi_kind == SOCKINFO_TCP)
        return IPPROTO_TCP;

    if(si->soi_kind == SOCKINFO_IN && si->soi_type == SOCK_DGRAM &&
       (!si->soi_protocol || si->soi_protocol == IPPROTO_UDP))
        return IPPROTO_UDP;

    return 0;
}

static inline bool local_sockets_macos_fill_endpoints(LOCAL_SOCKET *n, const struct in_sockinfo *ini) {
    if(!n || !ini)
        return false;

    if(ini->insi_vflag & INI_IPV4) {
        n->local.family = AF_INET;
        n->remote.family = AF_INET;
        n->local.ip.ipv4 = ini->insi_laddr.ina_46.i46a_addr4.s_addr;
        n->remote.ip.ipv4 = ini->insi_faddr.ina_46.i46a_addr4.s_addr;
    }
    else if(ini->insi_vflag & INI_IPV6) {
        n->local.family = AF_INET6;
        n->remote.family = AF_INET6;
        n->local.ip.ipv6 = ini->insi_laddr.ina_6;
        n->remote.ip.ipv6 = ini->insi_faddr.ina_6;
    }
    else
        return false;

    n->local.port = ntohs((uint16_t)ini->insi_lport);
    n->remote.port = ntohs((uint16_t)ini->insi_fport);
    return true;
}

struct local_sockets_macos_identity {
    pid_t pid;
    uint64_t process_start_sec;
    uint64_t process_start_usec;
    uint64_t generation;
    uint16_t protocol;
    uint16_t family;
    uint16_t local_port;
    uint16_t remote_port;
    union ipv46 local_ip;
    union ipv46 remote_ip;
};

static inline uint64_t local_sockets_macos_socket_identity(
    const LOCAL_SOCKET *n,
    const struct in_sockinfo *ini,
    const struct proc_bsdinfo *bsd)
{
    // Do not include the numeric FD: dup(2) aliases of one socket should
    // collapse like Linux inode-based records.
    struct local_sockets_macos_identity id = {
        .pid = n->pid,
        .process_start_sec = bsd ? bsd->pbi_start_tvsec : 0,
        .process_start_usec = bsd ? bsd->pbi_start_tvusec : 0,
        .generation = ini ? ini->insi_gencnt : 0,
        .protocol = n->local.protocol,
        .family = n->local.family,
        .local_port = n->local.port,
        .remote_port = n->remote.port,
        .local_ip = n->local.ip,
        .remote_ip = n->remote.ip,
    };

    uint64_t inode = XXH3_64bits(&id, sizeof(id));
    return inode ? inode : 1;
}

static inline bool local_sockets_macos_add_socket(
    LS_STATE *ls,
    pid_t pid,
    const struct proc_bsdinfo *bsd,
    const struct socket_fdinfo *sfi,
    STRING *cmdline)
{
    const struct socket_info *si = &sfi->psi;
    const struct in_sockinfo *ini = local_sockets_macos_in_sockinfo(si);
    uint16_t protocol = local_sockets_macos_socket_protocol(si);
    if(!ini || !protocol)
        return false;

    LOCAL_SOCKET n = {
        .direction = SOCKET_DIRECTION_NONE,
        .pid = pid,
        .ppid = bsd ? (pid_t)bsd->pbi_ppid : 0,
        .uid = bsd ? bsd->pbi_uid : UID_UNSET,
        .ipv6ony = { .checked = false, .ipv46 = false },
        .local = { .protocol = protocol },
        .remote = { .protocol = protocol },
    };

    if(!local_sockets_macos_fill_endpoints(&n, ini))
        return false;

    if(!local_sockets_macos_protocol_enabled(ls, n.local.family, protocol))
        return false;

    n.local.protocol = protocol;
    n.remote.protocol = protocol;

    if(protocol == IPPROTO_TCP)
        n.state = local_sockets_macos_normalize_tcp_state(si->soi_proto.pri_tcp.tcpsi_state);

    n.inode = local_sockets_macos_socket_identity(&n, ini, bsd);
    local_sockets_macos_set_comm(&n, bsd);

    // Suppress expected duplicate FD aliases before the shared add path logs a duplicate inode.
    XXH64_hash_t inode_hash = XXH3_64bits(&n.inode, sizeof(n.inode));
    SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl =
        simple_hashtable_get_slot_LOCAL_SOCKET(&ls->sockets_hashtable, inode_hash, &n.inode, false);
    if(SIMPLE_HASHTABLE_SLOT_DATA(sl))
        return false;

    if(cmdline)
        n.cmdline = string_dup(cmdline);

    if(!local_sockets_add_socket(ls, &n)) {
        string_freez(n.cmdline);
        return false;
    }

    return true;
}

static inline void local_sockets_macos_process_pid(LS_STATE *ls, pid_t pid) {
    if(pid <= 0)
        return;

    ls->stats.macos_pids_seen++;

    struct proc_bsdinfo bsd;
    bool have_bsd = local_sockets_macos_read_bsdinfo(pid, &bsd);
    if(have_bsd && bsd.pbi_pid && (pid_t)bsd.pbi_pid != pid)
        return;

    errno = 0;
    int fds_size = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, NULL, 0);
    if(fds_size <= 0) {
        int err = errno;
        if(local_sockets_macos_permission_errno(err))
            ls->stats.macos_pid_fds_permission_denied++;
        else if(err && !local_sockets_macos_process_race_errno(err))
            local_sockets_log(ls, "proc_pidinfo(PROC_PIDLISTFDS) size failed for PID %d: %s", pid, strerror(err));
        return;
    }

    struct proc_fdinfo *fds = NULL;
    int bytes_read = 0;
    bool fd_list_maybe_truncated = false;

    for(int attempt = 0; attempt < 3; attempt++) {
        int try_size = fds_size + fds_size / 5 + PROC_PIDLISTFD_SIZE;
        freez(fds);
        fds = mallocz((size_t)try_size);
        errno = 0;
        bytes_read = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, fds, try_size);
        if(bytes_read > 0 && bytes_read < try_size) {
            fd_list_maybe_truncated = false;
            break;
        }

        if(bytes_read <= 0) {
            int err = errno;
            if(local_sockets_macos_permission_errno(err))
                ls->stats.macos_pid_fds_permission_denied++;
            else if(err && !local_sockets_macos_process_race_errno(err))
                local_sockets_log(ls, "proc_pidinfo(PROC_PIDLISTFDS) failed for PID %d: %s", pid, strerror(err));
            freez(fds);
            return;
        }

        fd_list_maybe_truncated = true;
        fds_size = try_size * 2;
    }

    if(!fds || bytes_read <= 0) {
        freez(fds);
        return;
    }

    if(fd_list_maybe_truncated)
        ls->stats.macos_pid_fd_lists_maybe_truncated++;

    int fd_count = bytes_read / (int)PROC_PIDLISTFD_SIZE;
    char cmdline_buf[LOCAL_SOCKETS_CMDLINE_MAX];
    cmdline_buf[0] = '\0';
    STRING *cmdline = NULL;
    bool cmdline_checked = false;

    for(int i = 0; i < fd_count; i++) {
        if(fds[i].proc_fdtype != PROX_FDTYPE_SOCKET)
            continue;

        ls->stats.macos_socket_fds_seen++;
        struct socket_fdinfo sfi = { 0 };
        errno = 0;
        int rc = proc_pidfdinfo(pid, fds[i].proc_fd, PROC_PIDFDSOCKETINFO, &sfi, sizeof(sfi));
        if(rc != (int)sizeof(sfi)) {
            int err = errno;
            if(rc < 0 && local_sockets_macos_permission_errno(err))
                ls->stats.macos_socket_fds_permission_denied++;
            else if(rc < 0 && !local_sockets_macos_process_race_errno(err))
                local_sockets_log(
                    ls,
                    "proc_pidfdinfo(PROC_PIDFDSOCKETINFO) failed for PID %d FD %d: %s",
                    pid,
                    fds[i].proc_fd,
                    strerror(err));
            continue;
        }

        if(ls->config.cmdline && !cmdline_checked) {
            cmdline_checked = true;
            if(local_sockets_macos_read_cmdline(pid, cmdline_buf, sizeof(cmdline_buf))) {
                const char *trimmed = trim(cmdline_buf);
                if(trimmed && *trimmed)
                    cmdline = string_strdupz(trimmed);
            }
        }

        local_sockets_macos_add_socket(ls, pid, have_bsd ? &bsd : NULL, &sfi, cmdline);
    }

    string_freez(cmdline);
    freez(fds);
}

static inline void local_sockets_read_all_system_sockets(LS_STATE *ls) {
    int pids_size = proc_listpids(PROC_ALL_PIDS, 0, NULL, 0);
    if(pids_size <= 0) {
        local_sockets_log(ls, "proc_listpids(PROC_ALL_PIDS) size failed");
        return;
    }

    pid_t *pids = NULL;
    int bytes_read = 0;
    bool pid_list_maybe_truncated = false;

    for(int attempt = 0; attempt < 3; attempt++) {
        int try_size = pids_size + pids_size / 5 + (int)sizeof(pid_t);
        freez(pids);
        pids = mallocz((size_t)try_size);
        bytes_read = proc_listpids(PROC_ALL_PIDS, 0, pids, try_size);
        if(bytes_read > 0 && bytes_read < try_size) {
            pid_list_maybe_truncated = false;
            break;
        }

        if(bytes_read <= 0) {
            local_sockets_log(ls, "proc_listpids(PROC_ALL_PIDS) failed");
            freez(pids);
            return;
        }

        pid_list_maybe_truncated = true;
        pids_size = try_size * 2;
    }

    if(!pids || bytes_read <= 0) {
        freez(pids);
        return;
    }

    if(pid_list_maybe_truncated)
        ls->stats.macos_pid_lists_maybe_truncated++;

    int pid_count = bytes_read / (int)sizeof(pid_t);
    for(int i = 0; i < pid_count; i++)
        local_sockets_macos_process_pid(ls, pids[i]);

    local_sockets_macos_log_incomplete_summary(ls);
    freez(pids);
}

#endif /* NETDATA_LOCAL_SOCKETS_MACOS_H */
