// SPDX-License-Identifier: GPL-3.0-or-later

// FreeBSD socket collection backend for local-sockets.h.
// Included only when OS_FREEBSD is defined; never compiled standalone.

#ifndef NETDATA_LOCAL_SOCKETS_FREEBSD_H
#define NETDATA_LOCAL_SOCKETS_FREEBSD_H

#include <errno.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/sysctl.h>
#include <sys/user.h>
#include <sys/file.h>
// FreeBSD 14+ guards struct inpcb, struct xinpgen, and struct xtcpcb
// behind _KERNEL unless these macros are defined before the headers.
#ifndef _WANT_INPCB
#define _WANT_INPCB
#endif
#ifndef _WANT_TCPCB
#define _WANT_TCPCB
#endif

#include <netinet/in.h>
#include <netinet/in_pcb.h>
#include <netinet/tcp_var.h>
#include <netinet/tcp_fsm.h>

// --------------------------------------------------------------------------------------------------------------------
// kinfo_file address accessor: struct layout changed in FreeBSD 12.0.31

#if __FreeBSD_version >= 1200031
#define KF_SA_LOCAL(kf) ((const struct sockaddr_storage *)&(kf)->kf_un.kf_sock.kf_sa_local)
#define KF_SA_PEER(kf)  ((const struct sockaddr_storage *)&(kf)->kf_un.kf_sock.kf_sa_peer)
#else
#define KF_SA_LOCAL(kf) ((const struct sockaddr_storage *)&(kf)->kf_sa_local)
#define KF_SA_PEER(kf)  ((const struct sockaddr_storage *)&(kf)->kf_sa_peer)
#endif

// kf_sock_inpcb was removed when inpcb/tcpcb merged in FreeBSD 14.
#if __FreeBSD_version >= 1400074
#define KF_SOCK_PCB_TCP(kf) ((uint64_t)(uintptr_t)(kf)->kf_un.kf_sock.kf_sock_pcb)
#elif __FreeBSD_version >= 1200031
#define KF_SOCK_PCB_TCP(kf) ((uint64_t)(uintptr_t)(kf)->kf_un.kf_sock.kf_sock_inpcb)
#else
#define KF_SOCK_PCB_TCP(kf) ((uint64_t)(uintptr_t)(kf)->kf_sock_inpcb)
#endif

#if __FreeBSD_version >= 1200031
#define KF_SOCK_PCB_UDP(kf) ((uint64_t)(uintptr_t)(kf)->kf_un.kf_sock.kf_sock_pcb)
#else
#define KF_SOCK_PCB_UDP(kf) ((uint64_t)(uintptr_t)(kf)->kf_sock_pcb)
#endif

// --------------------------------------------------------------------------------------------------------------------
// Normalize FreeBSD TCPS_* states to Linux-compatible TCP_* values.
// This keeps the rest of the code (direction detection, TCP_STATE_2str display)
// unchanged, since Linux and FreeBSD use different numbering for the same states.

static inline int local_sockets_freebsd_normalize_tcp_state(int tcps) {
    switch (tcps) {
        case TCPS_ESTABLISHED:  return TCP_ESTABLISHED;
        case TCPS_SYN_SENT:     return TCP_SYN_SENT;
        case TCPS_SYN_RECEIVED: return TCP_SYN_RECV;
        case TCPS_FIN_WAIT_1:   return TCP_FIN_WAIT1;
        case TCPS_FIN_WAIT_2:   return TCP_FIN_WAIT2;
        case TCPS_TIME_WAIT:    return TCP_TIME_WAIT;
        case TCPS_CLOSED:       return TCP_CLOSE;
        case TCPS_CLOSE_WAIT:   return TCP_CLOSE_WAIT;
        case TCPS_LAST_ACK:     return TCP_LAST_ACK;
        case TCPS_LISTEN:       return TCP_LISTEN;
        case TCPS_CLOSING:      return TCP_CLOSING;
        default:                return 0;
    }
}

// --------------------------------------------------------------------------------------------------------------------
// PCB state table entry: keyed by 4-tuple for matching against kinfo_file data.

struct freebsd_tcp_state {
    uint8_t      family;
    uint16_t     local_port;
    uint16_t     remote_port;
    int          tcp_state;   // Linux-normalised TCP_* value
    union ipv46  local_ip;
    union ipv46  remote_ip;
};

// --------------------------------------------------------------------------------------------------------------------
// Fill a socket_endpoint from a sockaddr_storage.
// Returns false if the address family is not AF_INET / AF_INET6 (e.g., AF_UNSPEC for
// an unconnected peer slot).

static inline bool local_sockets_freebsd_fill_endpoint(
    struct socket_endpoint *ep,
    const struct sockaddr_storage *ss,
    uint16_t protocol)
{
    ep->protocol = protocol;

    if (ss->ss_family == AF_INET) {
        const struct sockaddr_in *sin = (const struct sockaddr_in *)ss;
        ep->family   = AF_INET;
        ep->port     = ntohs(sin->sin_port);
        ep->ip.ipv4  = sin->sin_addr.s_addr;
        return true;
    }
    if (ss->ss_family == AF_INET6) {
        const struct sockaddr_in6 *sin6 = (const struct sockaddr_in6 *)ss;
        ep->family   = AF_INET6;
        ep->port     = ntohs(sin6->sin6_port);
        ep->ip.ipv6  = sin6->sin6_addr;
        return true;
    }
    return false;
}

// --------------------------------------------------------------------------------------------------------------------
// Read net.inet.tcp.pcblist to build a table of {4-tuple → TCP state}.
// Returns a malloc'd array; caller must freez() it.
// FreeBSD 14+ changed the inpcb/tcpcb layout; if xt_tp is unavailable we skip
// the state table (direction detection still works via zero-peer-address check).

static struct freebsd_tcp_state *local_sockets_freebsd_read_tcp_pcblist(size_t *out_count) {
    *out_count = 0;

    size_t len = 0;
    if (sysctlbyname("net.inet.tcp.pcblist", NULL, &len, NULL, 0) < 0 || len == 0)
        return NULL;

    char *buf = NULL;
    // Retry loop: the list may grow between the size query and the read.
    for (int attempt = 0; attempt < 3; attempt++) {
        size_t try_len = len + len / 5;   // 20% headroom
        freez(buf);
        buf = mallocz(try_len);
        if (sysctlbyname("net.inet.tcp.pcblist", buf, &try_len, NULL, 0) == 0) {
            len = try_len;
            break;
        }
        if (errno != ENOMEM) {
            freez(buf);
            return NULL;
        }
    }
    if (!buf) return NULL;

    const char *p   = buf;
    const char *end = buf + len;

    // Parse header xinpgen to get the entry count.
    if ((size_t)(end - p) < sizeof(struct xinpgen)) {
        freez(buf);
        return NULL;
    }
    const struct xinpgen *hdr = (const struct xinpgen *)p;
    p += hdr->xig_len;

    size_t capacity = hdr->xig_count + 4;
    struct freebsd_tcp_state *states = mallocz(capacity * sizeof(*states));
    size_t count = 0;

    while (p + sizeof(struct xtcpcb) <= end) {
        const struct xtcpcb *tp = (const struct xtcpcb *)p;

        if (tp->xt_len == 0)
            break;
        // The footer is an xinpgen whose xt_len equals sizeof(xinpgen).
        if (tp->xt_len <= sizeof(struct xinpgen))
            break;

        const struct inpcb *inp = &tp->xt_inp;

        uint8_t family;
        if (inp->inp_vflag & INP_IPV6)
            family = AF_INET6;
        else if (inp->inp_vflag & INP_IPV4)
            family = AF_INET;
        else {
            p += tp->xt_len;
            continue;
        }

        if (count >= capacity) {
            capacity *= 2;
            states = reallocz(states, capacity * sizeof(*states));
        }

        struct freebsd_tcp_state *s = &states[count++];
        s->family      = family;
        s->local_port  = ntohs(inp->inp_lport);
        s->remote_port = ntohs(inp->inp_fport);

#if __FreeBSD_version < 1400000
        // FreeBSD 12/13: xt_tp.t_state is the TCP FSM state.
        s->tcp_state = local_sockets_freebsd_normalize_tcp_state(tp->xt_tp.t_state);
#else
        // FreeBSD 14+: inpcb/tcpcb merged; set a safe placeholder.
        // Direction detection still works via zero-peer-address check in
        // local_sockets_add_socket().
        s->tcp_state = TCP_ESTABLISHED;
#endif

        if (family == AF_INET) {
            s->local_ip.ipv4  = inp->inp_laddr.s_addr;
            s->remote_ip.ipv4 = inp->inp_faddr.s_addr;
        } else {
            s->local_ip.ipv6  = inp->in6p_laddr;
            s->remote_ip.ipv6 = inp->in6p_faddr;
        }

        p += tp->xt_len;
    }

    freez(buf);
    *out_count = count;
    return states;
}

// --------------------------------------------------------------------------------------------------------------------
// Lookup TCP state by 4-tuple (linear scan; O(n) per socket, n is typically < 5000).

static inline int local_sockets_freebsd_lookup_tcp_state(
    const struct freebsd_tcp_state *states,
    size_t n_states,
    uint8_t family,
    uint16_t local_port,
    const union ipv46 *local_ip,
    uint16_t remote_port,
    const union ipv46 *remote_ip)
{
    for (size_t i = 0; i < n_states; i++) {
        const struct freebsd_tcp_state *s = &states[i];

        if (s->family != family || s->local_port != local_port || s->remote_port != remote_port)
            continue;

        if (family == AF_INET) {
            if (s->local_ip.ipv4 != local_ip->ipv4 || s->remote_ip.ipv4 != remote_ip->ipv4)
                continue;
        } else {
            if (memcmp(&s->local_ip.ipv6,  local_ip,  sizeof(struct in6_addr)) != 0 ||
                memcmp(&s->remote_ip.ipv6, remote_ip, sizeof(struct in6_addr)) != 0)
                continue;
        }

        return s->tcp_state;
    }
    // Not found in pcblist: treat as established so the socket still appears.
    return TCP_ESTABLISHED;
}

// --------------------------------------------------------------------------------------------------------------------
// Walk all processes + their file descriptors via sysctl(KERN_PROC_FILEDESC).
// For each TCP/UDP socket FD, look up TCP state from the pcblist table, build a
// LOCAL_SOCKET, and feed it into local_sockets_add_socket().

static inline void local_sockets_freebsd_enumerate_pids(
    LS_STATE *ls,
    const struct freebsd_tcp_state *tcp_states,
    size_t n_tcp_states)
{
    // --- Get list of all processes ---
    int mib_all[3] = { CTL_KERN, KERN_PROC, KERN_PROC_ALL };
    size_t proc_size = 0;

    if (sysctl(mib_all, 3, NULL, &proc_size, NULL, 0) < 0) {
        local_sockets_log(ls, "sysctl KERN_PROC_ALL size failed: %s", strerror(errno));
        return;
    }

    struct kinfo_proc *procs = NULL;
    size_t nprocs = 0;

    for (int attempt = 0; attempt < 3; attempt++) {
        size_t try_size = proc_size + proc_size / 5;
        freez(procs);
        procs = mallocz(try_size);
        if (sysctl(mib_all, 3, procs, &try_size, NULL, 0) == 0) {
            nprocs = try_size / sizeof(struct kinfo_proc);
            break;
        }
        if (errno != ENOMEM) {
            freez(procs);
            local_sockets_log(ls, "sysctl KERN_PROC_ALL failed: %s", strerror(errno));
            return;
        }
    }
    if (!procs) return;

    // --- Walk each process's file descriptors ---
    for (size_t i = 0; i < nprocs; i++) {
        pid_t pid  = procs[i].ki_pid;
        pid_t ppid = procs[i].ki_ppid;
        uid_t uid  = procs[i].ki_uid;

        if (pid <= 0) continue;

        int mib_fd[4] = { CTL_KERN, KERN_PROC, KERN_PROC_FILEDESC, (int)pid };
        size_t fd_size = 0;

        if (sysctl(mib_fd, 4, NULL, &fd_size, NULL, 0) < 0 || fd_size == 0)
            continue;

        char *fdbuf = NULL;
        for (int attempt = 0; attempt < 3; attempt++) {
            size_t try_size = fd_size + fd_size / 5;
            freez(fdbuf);
            fdbuf = mallocz(try_size);
            if (sysctl(mib_fd, 4, fdbuf, &try_size, NULL, 0) == 0) {
                fd_size = try_size;
                break;
            }
            if (errno != ENOMEM) {
                freez(fdbuf);
                fdbuf = NULL;
                break;
            }
        }
        if (!fdbuf) continue;

        char cmdline[LOCAL_SOCKETS_CMDLINE_MAX];
        bool cmdline_loaded = false;
        cmdline[0] = '\0';

        char *p   = fdbuf;
        char *efdbuf = fdbuf + fd_size;

        while (p < efdbuf) {
            struct kinfo_file *kf = (struct kinfo_file *)p;
            if (kf->kf_structsize == 0) break;

            // Only care about internet sockets owned by a FD (fd >= 0).
            if (kf->kf_fd < 0 ||
                kf->kf_type != KF_TYPE_SOCKET ||
                (kf->kf_sock_domain != AF_INET && kf->kf_sock_domain != AF_INET6) ||
                (kf->kf_sock_protocol != IPPROTO_TCP && kf->kf_sock_protocol != IPPROTO_UDP)) {
                p += kf->kf_structsize;
                continue;
            }

            const struct sockaddr_storage *sa_local = KF_SA_LOCAL(kf);
            const struct sockaddr_storage *sa_peer  = KF_SA_PEER(kf);

            // Build a LOCAL_SOCKET from the kinfo_file data.
            LOCAL_SOCKET n = {
                .direction = SOCKET_DIRECTION_NONE,
                .uid       = uid,
                .pid       = pid,
                .ppid      = ppid,
                .ipv6ony   = { .checked = false, .ipv46 = false },
            };

            if (!local_sockets_freebsd_fill_endpoint(&n.local, sa_local, kf->kf_sock_protocol)) {
                p += kf->kf_structsize;
                continue;
            }

            // Peer may be unset (listening / unconnected UDP); zero it out in that case.
            if (!local_sockets_freebsd_fill_endpoint(&n.remote, sa_peer, kf->kf_sock_protocol)) {
                n.remote.family   = kf->kf_sock_domain;
                n.remote.protocol = kf->kf_sock_protocol;
                n.remote.port     = 0;
                memset(&n.remote.ip, 0, sizeof(n.remote.ip));
            }

            // Use PCB kernel address as the unique per-socket key (inode equivalent).
            if (kf->kf_sock_protocol == IPPROTO_TCP)
                n.inode = KF_SOCK_PCB_TCP(kf);
            else
                n.inode = KF_SOCK_PCB_UDP(kf);

            if (!n.inode) {
                p += kf->kf_structsize;
                continue;
            }

            // TCP state from pcblist; UDP is stateless.
            if (kf->kf_sock_protocol == IPPROTO_TCP) {
                n.state = local_sockets_freebsd_lookup_tcp_state(
                    tcp_states, n_tcp_states,
                    n.local.family,
                    n.local.port,
                    &n.local.ip,
                    n.remote.port,
                    &n.remote.ip);
            }

            // Lazily read cmdline on first socket found for this process.
            if (ls->config.cmdline && !cmdline_loaded) {
                cmdline_loaded = true;
                int mib_args[4] = { CTL_KERN, KERN_PROC, KERN_PROC_ARGS, (int)pid };
                size_t args_size = sizeof(cmdline) - 1;
                if (sysctl(mib_args, 4, cmdline, &args_size, NULL, 0) == 0 && args_size > 0) {
                    // cmdline args are NUL-separated; convert to spaces.
                    for (size_t j = 0; j + 1 < args_size; j++)
                        if (cmdline[j] == '\0') cmdline[j] = ' ';
                    cmdline[args_size] = '\0';
                    local_sockets_fix_cmdline(cmdline);
                }
                else
                    cmdline[0] = '\0';
            }

            // --- Register PID attribution entry ---
            {
                XXH64_hash_t h = XXH3_64bits(&n.inode, sizeof(n.inode));
                SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl =
                    simple_hashtable_get_slot_PID_SOCKET(&ls->pid_sockets_hashtable, h, &n.inode, true);
                struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl);

                if (!ps || (ps->pid == 1 && pid != 1)) {
                    if (!ps) ps = aral_callocz(ls->pid_socket_aral);

                    ps->inode        = n.inode;
                    ps->pid          = pid;
                    ps->ppid         = ppid;
                    ps->uid          = uid;
                    ps->net_ns_inode = 0;   // FreeBSD: no network namespaces

                    if (ls->config.comm)
                        strncpyz(ps->comm, procs[i].ki_comm, sizeof(ps->comm) - 1);

                    if (ls->config.cmdline) {
                        freez(ps->cmdline);
                        const char *t = cmdline[0] ? trim(cmdline) : NULL;
                        ps->cmdline = t ? strdupz(t) : NULL;
                    }

                    simple_hashtable_set_slot_PID_SOCKET(&ls->pid_sockets_hashtable, sl, h, ps);
                }
            }

            // Comm goes into the socket via pid_socket lookup inside add_socket;
            // set it here too so it is available even if the pid_socket slot lost a race.
            if (ls->config.comm)
                strncpyz(n.comm, procs[i].ki_comm, sizeof(n.comm) - 1);

            local_sockets_add_socket(ls, &n);

            p += kf->kf_structsize;
        }

        freez(fdbuf);
    }

    freez(procs);
}

// --------------------------------------------------------------------------------------------------------------------
// FreeBSD entry point for local_sockets_read_all_system_sockets().
// Called from local_sockets_process() in local-sockets.h.

static inline void local_sockets_read_all_system_sockets(LS_STATE *ls) {
    // Phase 1: build TCP state table from net.inet.tcp.pcblist.
    size_t n_tcp_states = 0;
    struct freebsd_tcp_state *tcp_states = NULL;

    if (ls->config.tcp4 || ls->config.tcp6)
        tcp_states = local_sockets_freebsd_read_tcp_pcblist(&n_tcp_states);

    // Phase 2: enumerate per-process sockets via KERN_PROC_FILEDESC.
    local_sockets_freebsd_enumerate_pids(ls, tcp_states, n_tcp_states);

    freez(tcp_states);
}

#endif /* NETDATA_LOCAL_SOCKETS_FREEBSD_H */
