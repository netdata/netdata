// SPDX-License-Identifier: GPL-3.0-or-later

// Windows socket collection backend for local-sockets.h.
// Included only when OS_WINDOWS is defined; never compiled standalone.

#ifndef NETDATA_LOCAL_SOCKETS_WINDOWS_H
#define NETDATA_LOCAL_SOCKETS_WINDOWS_H

#include <windows.h>
#include <tlhelp32.h>
#include "libnetdata/os/system-maps/cached-sid-username.h"

// Minimal IP Helper API definitions.
// <winsock2.h> and <ws2tcpip.h> cannot be included here: libnetdata.h already
// pulls in POSIX socket headers (via uv.h), and the Windows headers redefine
// hostent, sockaddr, pollfd etc. causing compile errors on Cygwin/MSYS2.
// Base types come from <windows.h>, which is included for OS_WINDOWS by
// libnetdata/common.h. GetExtendedTcpTable/GetExtendedUdpTable are linked
// through iphlpapi (see libnetdata's Windows link libraries).

// MIB TCP states (iprtrmib.h values), declared here to avoid including
// <iprtrmib.h>/<winsock2.h> in this TU.
#ifndef MIB_TCP_STATE_CLOSED
#define MIB_TCP_STATE_CLOSED     1
#define MIB_TCP_STATE_LISTEN     2
#define MIB_TCP_STATE_SYN_SENT   3
#define MIB_TCP_STATE_SYN_RCVD   4
#define MIB_TCP_STATE_ESTAB      5
#define MIB_TCP_STATE_FIN_WAIT1  6
#define MIB_TCP_STATE_FIN_WAIT2  7
#define MIB_TCP_STATE_CLOSE_WAIT 8
#define MIB_TCP_STATE_CLOSING    9
#define MIB_TCP_STATE_LAST_ACK  10
#define MIB_TCP_STATE_TIME_WAIT 11
#define MIB_TCP_STATE_DELETE_TCB 12
#endif

#ifndef MIB_TCPROW_OWNER_PID
typedef struct {
    DWORD dwState;
    DWORD dwLocalAddr;
    DWORD dwLocalPort;
    DWORD dwRemoteAddr;
    DWORD dwRemotePort;
    DWORD dwOwningPid;
} MIB_TCPROW_OWNER_PID;

typedef struct {
    UCHAR ucLocalAddr[16];
    DWORD dwLocalScopeId;
    DWORD dwLocalPort;
    UCHAR ucRemoteAddr[16];
    DWORD dwRemoteScopeId;
    DWORD dwRemotePort;
    DWORD dwState;
    DWORD dwOwningPid;
} MIB_TCP6ROW_OWNER_PID;

typedef struct {
    DWORD dwNumEntries;
    MIB_TCPROW_OWNER_PID table[];
} MIB_TCPTABLE_OWNER_PID;

typedef struct {
    DWORD dwNumEntries;
    MIB_TCP6ROW_OWNER_PID table[];
} MIB_TCP6TABLE_OWNER_PID;

typedef struct {
    DWORD dwLocalAddr;
    DWORD dwLocalPort;
    DWORD dwOwningPid;
} MIB_UDPROW_OWNER_PID;

typedef struct {
    UCHAR ucLocalAddr[16];
    DWORD dwLocalScopeId;
    DWORD dwLocalPort;
    DWORD dwOwningPid;
} MIB_UDP6ROW_OWNER_PID;

typedef struct {
    DWORD dwNumEntries;
    MIB_UDPROW_OWNER_PID table[];
} MIB_UDPTABLE_OWNER_PID;

typedef struct {
    DWORD dwNumEntries;
    MIB_UDP6ROW_OWNER_PID table[];
} MIB_UDP6TABLE_OWNER_PID;

#endif // MIB_TCPROW_OWNER_PID

// Windows-native AF_ values for the IP Helper API calls.
// Cygwin POSIX headers define AF_INET6=10; the Windows API expects 23.
// AF_INET=2 happens to be the same on both.
#define LS_WIN_AF_INET  2
#define LS_WIN_AF_INET6 23

typedef enum { TCP_TABLE_OWNER_PID_ALL = 5 } LS_TCP_TABLE_CLASS;
typedef enum { UDP_TABLE_OWNER_PID = 1 }    LS_UDP_TABLE_CLASS;

DWORD WINAPI GetExtendedTcpTable(PVOID pTcpTable, PDWORD pdwSize, BOOL bOrder,
                                 ULONG ulAf, LS_TCP_TABLE_CLASS TableClass, ULONG Reserved);
DWORD WINAPI GetExtendedUdpTable(PVOID pUdpTable, PDWORD pdwSize, BOOL bOrder,
                                 ULONG ulAf, LS_UDP_TABLE_CLASS TableClass, ULONG Reserved);

// --------------------------------------------------------------------------------------------------------------------
// per-collection-pass process table (Toolhelp snapshot):
// pid -> ppid, exe base name, SID-account username (resolved lazily and cached)

#define LS_WIN_COMM_MAX 256
#define LS_WIN_USERNAME_MAX 256

typedef struct {
    DWORD pid;
    DWORD ppid;
    bool valid;
    char comm[LS_WIN_COMM_MAX];
    char username[LS_WIN_USERNAME_MAX];
} LS_WINDOWS_PROC;

static inline void ls_win_resolve_username(DWORD pid, char *dst, size_t dst_size) {
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';

    // idempotent and thread-safe; safe to call on every resolution
    cached_sid_username_init();

    HANDLE hp = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid);
    if(!hp)
        return;

    HANDLE ht = NULL;
    if(!OpenProcessToken(hp, TOKEN_QUERY, &ht)) {
        CloseHandle(hp);
        return;
    }

    DWORD sz = 0;
    GetTokenInformation(ht, TokenUser, NULL, 0, &sz);
    if(sz == 0) {
        CloseHandle(ht);
        CloseHandle(hp);
        return;
    }

    TOKEN_USER *tu = mallocz(sz);
    BOOL ok = GetTokenInformation(ht, TokenUser, tu, sz, &sz);
    CloseHandle(ht);
    CloseHandle(hp);
    if(ok) {
        STRING *s = cached_sid_fullname_or_sid_str(tu->User.Sid);
        if(s) {
            const char *name = string2str(s);
            if(name && *name)
                snprintfz(dst, dst_size, "%s", name);
            string_freez(s);
        }
    }
    freez(tu);
}

static inline bool ls_win_proc_table_build(LS_WINDOWS_PROC **out, size_t *out_count) {
    *out = NULL;
    *out_count = 0;

    HANDLE snap = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if(snap == INVALID_HANDLE_VALUE)
        return false;

    size_t count = 0;
    size_t capacity = 0;
    LS_WINDOWS_PROC *procs = NULL;

    PROCESSENTRY32W pe;
    pe.dwSize = sizeof(pe);
    if(Process32FirstW(snap, &pe)) {
        do {
            if(count >= capacity) {
                capacity = capacity ? capacity * 2 : 256;
                procs = reallocz(procs, capacity * sizeof(*procs));
            }

            LS_WINDOWS_PROC *p = &procs[count];
            memset(p, 0, sizeof(*p));
            p->pid = pe.th32ProcessID;
            p->ppid = pe.th32ParentProcessID;
            p->valid = true;
            if(utf16_to_utf8(p->comm, sizeof(p->comm) - 1, pe.szExeFile, -1, NULL) == 0)
                p->comm[0] = '\0';
            count++;
        } while(Process32NextW(snap, &pe));
    }

    CloseHandle(snap);

    if(count == 0) {
        freez(procs);
        return false;
    }

    *out = procs;
    *out_count = count;
    return true;
}

static inline const LS_WINDOWS_PROC *ls_win_proc_table_lookup(
    LS_WINDOWS_PROC *procs, size_t count, DWORD pid, char *username, size_t username_size)
{
    for(size_t i = 0; i < count; i++) {
        if(procs[i].valid && procs[i].pid == pid) {
            if(username && username_size) {
                username[0] = '\0';
                if(!procs[i].username[0])
                    ls_win_resolve_username(pid, procs[i].username, sizeof(procs[i].username) - 1);
                snprintfz(username, username_size, "%s", procs[i].username);
            }
            return &procs[i];
        }
    }
    return NULL;
}

// enrich one socket from the process table: ppid, comm, username, win_comm
static inline void ls_win_proc_enrich(LS_STATE *ls, LOCAL_SOCKET *n, DWORD pid, LS_WINDOWS_PROC *procs, size_t proc_count) {
    n->uid = UID_UNSET;
    n->net_ns_inode = 0;
    n->cmdline = NULL;
    n->direction = SOCKET_DIRECTION_NONE;
    n->win_comm[0] = '\0';
    n->win_username[0] = '\0';

    // username transport: value-copied into the OS_WINDOWS LOCAL_SOCKET members
    const LS_WINDOWS_PROC *p = ls_win_proc_table_lookup(
        procs, proc_count, pid, n->win_username, sizeof(n->win_username) - 1);

    if(p) {
        n->ppid = p->ppid;
        if(ls->config.comm) {
            snprintfz(n->comm, sizeof(n->comm) - 1, "%s", p->comm);
            snprintfz(n->win_comm, sizeof(n->win_comm) - 1, "%s", p->comm);
        }
    }
}

// --------------------------------------------------------------------------------------------------------------------
// MIB TCP state -> Linux-compatible TCP_* values (see local-sockets.h)

static inline int ls_win_tcp_state_to_linux(DWORD mib_state) {
    switch(mib_state) {
        case MIB_TCP_STATE_LISTEN:     return TCP_LISTEN;
        case MIB_TCP_STATE_ESTAB:      return TCP_ESTABLISHED;
        case MIB_TCP_STATE_SYN_SENT:   return TCP_SYN_SENT;
        case MIB_TCP_STATE_SYN_RCVD:   return TCP_SYN_RECV;
        case MIB_TCP_STATE_FIN_WAIT1:  return TCP_FIN_WAIT1;
        case MIB_TCP_STATE_FIN_WAIT2:  return TCP_FIN_WAIT2;
        case MIB_TCP_STATE_CLOSE_WAIT: return TCP_CLOSE_WAIT;
        case MIB_TCP_STATE_CLOSING:    return TCP_CLOSING;
        case MIB_TCP_STATE_LAST_ACK:   return TCP_LAST_ACK;
        case MIB_TCP_STATE_TIME_WAIT:  return TCP_TIME_WAIT;
        case MIB_TCP_STATE_CLOSED:     return TCP_CLOSE;
        case MIB_TCP_STATE_DELETE_TCB: return TCP_CLOSE;
        default:                       return TCP_CLOSE;
    }
}

static inline void ls_win_fill_endpoint_ipv4(struct socket_endpoint *ep, DWORD addr, DWORD port, uint16_t protocol) {
    ep->protocol = protocol;
    ep->family = AF_INET;
    ep->port = ntohs((uint16_t)port);
    ep->ip.ipv4 = addr; // network byte order, same as struct in_addr
}

static inline void ls_win_fill_endpoint_ipv6(struct socket_endpoint *ep, const UCHAR addr[16], DWORD port, uint16_t protocol) {
    ep->protocol = protocol;
    ep->family = AF_INET6;
    ep->port = ntohs((uint16_t)port);
    memcpy(&ep->ip.ipv6, addr, sizeof(ep->ip.ipv6));
}

// --------------------------------------------------------------------------------------------------------------------
// table fetching with ERROR_INSUFFICIENT_BUFFER retry.
// Sockets can be created between the size probe and the fetch on a busy host,
// so the second call may still report ERROR_INSUFFICIENT_BUFFER; retry once
// with the size the API reports before giving up.

#define LS_WIN_DEFINE_TABLE_FETCH(name, fn, table_class)                     \
    static inline void *name(ULONG af) {                                     \
        DWORD size = 0;                                                      \
        DWORD rc = fn(NULL, &size, FALSE, af, table_class, 0);               \
        if(rc != ERROR_INSUFFICIENT_BUFFER || size == 0)                     \
            return NULL;                                                     \
        void *table = mallocz(size);                                         \
        rc = fn(table, &size, FALSE, af, table_class, 0);                    \
        if(rc == ERROR_INSUFFICIENT_BUFFER) {                                \
            table = reallocz(table, size);                                   \
            rc = fn(table, &size, FALSE, af, table_class, 0);                \
        }                                                                    \
        if(rc != 0) {                                                      \
            freez(table);                                                    \
            return NULL;                                                     \
        }                                                                    \
        return table;                                                        \
    }

LS_WIN_DEFINE_TABLE_FETCH(ls_win_fetch_tcp_table, GetExtendedTcpTable, TCP_TABLE_OWNER_PID_ALL)
LS_WIN_DEFINE_TABLE_FETCH(ls_win_fetch_udp_table, GetExtendedUdpTable, UDP_TABLE_OWNER_PID)

// --------------------------------------------------------------------------------------------------------------------
// platform backend entry point

static inline void local_sockets_read_all_system_sockets(LS_STATE *ls) {
    LS_WINDOWS_PROC *procs = NULL;
    size_t proc_count = 0;
    ls_win_proc_table_build(&procs, &proc_count);

    uint64_t counter = 0;

    // TCP IPv4
    if(ls->config.tcp4) {
        MIB_TCPTABLE_OWNER_PID *t = (MIB_TCPTABLE_OWNER_PID *)ls_win_fetch_tcp_table(LS_WIN_AF_INET);
        if(t) {
            for(DWORD i = 0; i < t->dwNumEntries; i++) {
                MIB_TCPROW_OWNER_PID *r = &t->table[i];

                LOCAL_SOCKET n = { 0 };
                n.inode = ++counter; // synthetic unique per-snapshot inode
                n.pid = r->dwOwningPid;
                n.state = ls_win_tcp_state_to_linux(r->dwState);
                ls_win_fill_endpoint_ipv4(&n.local, r->dwLocalAddr, r->dwLocalPort, IPPROTO_TCP);
                ls_win_fill_endpoint_ipv4(&n.remote, r->dwRemoteAddr, r->dwRemotePort, IPPROTO_TCP);
                ls_win_proc_enrich(ls, &n, r->dwOwningPid, procs, proc_count);

                local_sockets_add_socket(ls, &n);
            }
            freez(t);
        }
    }

    // TCP IPv6
    if(ls->config.tcp6) {
        MIB_TCP6TABLE_OWNER_PID *t = (MIB_TCP6TABLE_OWNER_PID *)ls_win_fetch_tcp_table(LS_WIN_AF_INET6);
        if(t) {
            for(DWORD i = 0; i < t->dwNumEntries; i++) {
                MIB_TCP6ROW_OWNER_PID *r = &t->table[i];

                LOCAL_SOCKET n = { 0 };
                n.inode = ++counter;
                n.pid = r->dwOwningPid;
                n.state = ls_win_tcp_state_to_linux(r->dwState);
                ls_win_fill_endpoint_ipv6(&n.local, r->ucLocalAddr, r->dwLocalPort, IPPROTO_TCP);
                ls_win_fill_endpoint_ipv6(&n.remote, r->ucRemoteAddr, r->dwRemotePort, IPPROTO_TCP);
                ls_win_proc_enrich(ls, &n, r->dwOwningPid, procs, proc_count);

                local_sockets_add_socket(ls, &n);
            }
            freez(t);
        }
    }

    // UDP IPv4
    if(ls->config.udp4) {
        MIB_UDPTABLE_OWNER_PID *t = (MIB_UDPTABLE_OWNER_PID *)ls_win_fetch_udp_table(LS_WIN_AF_INET);
        if(t) {
            for(DWORD i = 0; i < t->dwNumEntries; i++) {
                MIB_UDPROW_OWNER_PID *r = &t->table[i];

                LOCAL_SOCKET n = { 0 };
                n.inode = ++counter;
                n.pid = r->dwOwningPid;
                n.state = 0;
                ls_win_fill_endpoint_ipv4(&n.local, r->dwLocalAddr, r->dwLocalPort, IPPROTO_UDP);
                n.remote.family = AF_INET;
                n.remote.protocol = IPPROTO_UDP;
                n.remote.port = 0;
                memset(&n.remote.ip, 0, sizeof(n.remote.ip));
                ls_win_proc_enrich(ls, &n, r->dwOwningPid, procs, proc_count);

                local_sockets_add_socket(ls, &n);
            }
            freez(t);
        }
    }

    // UDP IPv6
    if(ls->config.udp6) {
        MIB_UDP6TABLE_OWNER_PID *t = (MIB_UDP6TABLE_OWNER_PID *)ls_win_fetch_udp_table(LS_WIN_AF_INET6);
        if(t) {
            for(DWORD i = 0; i < t->dwNumEntries; i++) {
                MIB_UDP6ROW_OWNER_PID *r = &t->table[i];

                LOCAL_SOCKET n = { 0 };
                n.inode = ++counter;
                n.pid = r->dwOwningPid;
                n.state = 0;
                ls_win_fill_endpoint_ipv6(&n.local, r->ucLocalAddr, r->dwLocalPort, IPPROTO_UDP);
                n.remote.family = AF_INET6;
                n.remote.protocol = IPPROTO_UDP;
                n.remote.port = 0;
                memset(&n.remote.ip, 0, sizeof(n.remote.ip));
                ls_win_proc_enrich(ls, &n, r->dwOwningPid, procs, proc_count);

                local_sockets_add_socket(ls, &n);
            }
            freez(t);
        }
    }

    if(procs)
        freez(procs);
}

#endif /* NETDATA_LOCAL_SOCKETS_WINDOWS_H */
