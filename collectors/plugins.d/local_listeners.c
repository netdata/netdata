// SPDX-License-Identifier: GPL-3.0-or-later

#include "local-sockets.h"
#include "libnetdata/required_dummies.h"

// --------------------------------------------------------------------------------------------------------------------

static const char *protocol_name(LOCAL_SOCKET *n) {
    if(n->family == AF_INET) {
        if(n->protocol == IPPROTO_TCP)
            return "TCP";
        else if(n->protocol == IPPROTO_UDP)
            return "UDP";
        else
            return "UNKNOWN_IPV4";
    }
    else if(n->family == AF_INET6) {
        if (n->protocol == IPPROTO_TCP)
            return "TCP6";
        else if(n->protocol == IPPROTO_UDP)
            return "UDP6";
        else
            return "UNKNOWN_IPV6";
    }
    else
        return "UNKNOWN";
}

static void print_local_listeners(LS_STATE *ls __maybe_unused, LOCAL_SOCKET *n, void *data __maybe_unused) {
    char local_address[INET6_ADDRSTRLEN];
    char remote_address[INET6_ADDRSTRLEN];

    if(n->family == AF_INET) {
        ipv4_address_to_txt(n->local.ip.ipv4, local_address);
        ipv4_address_to_txt(n->remote.ip.ipv4, remote_address);
    }
    else if(n->family == AF_INET6) {
        ipv6_address_to_txt(&n->local.ip.ipv6, local_address);
        ipv6_address_to_txt(&n->remote.ip.ipv6, remote_address);
    }

    printf("%s|%s|%u|%s\n", protocol_name(n), local_address, n->local.port, n->cmdline ? n->cmdline : "");
}

static void print_local_listeners_debug(LS_STATE *ls __maybe_unused, LOCAL_SOCKET *n, void *data __maybe_unused) {
    char local_address[INET6_ADDRSTRLEN];
    char remote_address[INET6_ADDRSTRLEN];

    if(n->family == AF_INET) {
        ipv4_address_to_txt(n->local.ip.ipv4, local_address);
        ipv4_address_to_txt(n->remote.ip.ipv4, remote_address);
    }
    else if(n->family == AF_INET6) {
        ipv6_address_to_txt(&n->local.ip.ipv6, local_address);
        ipv6_address_to_txt(&n->remote.ip.ipv6, remote_address);
    }

    printf("%s, direction=%s%s%s%s%s pid=%d, state=0x%0x, ns=%"PRIu64", local=%s[:%u], remote=%s[:%u], comm=%s\n",
           protocol_name(n),
           (n->direction & SOCKET_DIRECTION_LISTEN) ? "LISTEN," : "",
           (n->direction & SOCKET_DIRECTION_INBOUND) ? "INBOUND," : "",
           (n->direction & SOCKET_DIRECTION_OUTBOUND) ? "OUTBOUND," : "",
           (n->direction & SOCKET_DIRECTION_LOCAL) ? "LOCAL," : "",
           (n->direction == 0) ? "NONE," : "",
           n->pid,
           n->state,
           n->net_ns_inode,
           local_address, n->local.port,
           remote_address, n->remote.port,
           n->comm);
}

// --------------------------------------------------------------------------------------------------------------------

int main(int argc, char **argv) {
    LS_STATE ls = {
        .config = {
            .listening = true,
            .inbound = false,
            .outbound = false,
            .local = false,
            .tcp4 = true,
            .tcp6 = true,
            .udp4 = true,
            .udp6 = true,
            .pid = false,
            .cmdline = true,
            .comm = false,
            .namespaces = true,

            .max_errors = 10,

            .cb = print_local_listeners,
            .data = NULL,
        },
        .stats = { 0 },
        .sockets_hashtable = { 0 },
        .local_ips_hashtable = { 0 },
        .listening_ports_hashtable = { 0 },
    };

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(!netdata_configured_host_prefix) netdata_configured_host_prefix = "";

    for (int i = 1; i < argc; i++) {
        char *s = argv[i];
        bool positive = true;

        if(strcmp(s, "-h") == 0 || strcmp(s, "--help") == 0) {
            fprintf(stderr,
                    "\n"
                    " Netdata local-listeners\n"
                    " (C) 2024 Netdata Inc.\n"
                    "\n"
                    " This program prints a list of all the processes that have a listening socket.\n"
                    " It is used by Netdata to auto-detect the services running.\n"
                    "\n"
                    " Options:\n"
                    "\n"
                    " The options:\n"
                    "\n"
                    "    udp, udp4, udp6, tcp, tcp4, tcp6, ipv4, ipv6\n"
                    "\n"
                    " select the sources to read currently available sockets.\n"
                    "\n"
                    " while:\n"
                    "\n"
                    "    listening, local, inbound, outbound, namespaces\n"
                    "\n"
                    " filter the output based on the direction of the sockets.\n"
                    "\n"
                    " Prepending any option with 'no-', 'not-' or 'non-' will disable them.\n"
                    "\n"
                    " Current options:\n"
                    "\n"
                    "    %s %s %s %s %s %s %s %s %s\n"
                    "\n"
                    " Option 'debug' enables all sources and all directions and provides\n"
                    " a full dump of current sockets.\n"
                    "\n"
                    " DIRECTION DETECTION\n"
                    " The program detects the direction of the sockets using these rules:\n"
                    "\n"
                    "   - listening   are all the TCP sockets that are in listen state\n"
                    "                 and all sockets that their remote IP is zero.\n"
                    "\n"
                    "   - local       are all the non-listening sockets that either their source IP\n"
                    "                 or their remote IP are loopback addresses. Loopback addresses are\n"
                    "                 those in 127.0.0.0/8 and ::1. When IPv4 addresses are mapped\n"
                    "                 into IPv6, the program extracts the IPv4 addresses to check them.\n"
                    "\n"
                    "                 Also, local are considered all the sockets that their remote\n"
                    "                 IP is one of the IPs that appear as local on another socket.\n"
                    "\n"
                    "   - inbound     are all the non-listening and non-local sockets that their local\n"
                    "                 port is a port of another socket that is marked as listening.\n"
                    "\n"
                    "   - outbound    are all the other sockets.\n"
                    "\n"
                    " Keep in mind that this kind of socket direction detection is not 100%% accurate,\n"
                    " and there may be cases (e.g. reusable sockets) that this code may incorrectly\n"
                    " mark sockets as inbound or outbound.\n"
                    "\n"
                    " WARNING:\n"
                    " This program reads the entire /proc/net/{tcp,udp,tcp6,upd6} files, builds\n"
                    " multiple hash maps in memory and traverses the entire /proc filesystem to\n"
                    " associate sockets with processes. We have made the most to make it as\n"
                    " lightweight and fast as possible, but still this program has a lot of work\n"
                    " to do and it may have some impact on very busy servers with millions of.\n"
                    " established connections."
                    "\n"
                    " Therefore, we suggest to avoid running it repeatedly for data collection.\n"
                    "\n"
                    " Netdata executes it only when it starts to auto-detect data collection sources\n"
                    " and initialize the network dependencies explorer."
                    "\n"
                    , ls.config.udp4 ? "udp4" :"no-udp4"
                    , ls.config.udp6 ? "udp6" :"no-udp6"
                    , ls.config.tcp4 ? "tcp4" :"no-tcp4"
                    , ls.config.tcp6 ? "tcp6" :"no-tcp6"
                    , ls.config.listening ? "listening" : "no-listening"
                    , ls.config.local ? "local" : "no-local"
                    , ls.config.inbound ? "inbound" : "no-inbound"
                    , ls.config.outbound ? "outbound" : "no-outbound"
                    , ls.config.namespaces ? "namespaces" : "no-namespaces"
                    );
            exit(1);
        }

        if(strncmp(s, "no-", 3) == 0) {
            positive = false;
            s += 3;
        }
        else if(strncmp(s, "not-", 4) == 0 || strncmp(s, "non-", 4) == 0) {
            positive = false;
            s += 4;
        }

        if(strcmp(s, "debug") == 0 || strcmp(s, "--debug") == 0) {
            fprintf(stderr, "%s debugging\n", positive ? "enabling" : "disabling");
            ls.config.listening = true;
            ls.config.local = true;
            ls.config.inbound = true;
            ls.config.outbound = true;
            ls.config.pid = true;
            ls.config.comm = true;
            ls.config.cmdline = true;
            ls.config.namespaces = true;
            ls.config.max_errors = SIZE_MAX;
            ls.config.cb = print_local_listeners_debug;
        }
        else if (strcmp("tcp", s) == 0) {
            ls.config.tcp4 = ls.config.tcp6 = positive;
            // fprintf(stderr, "%s tcp4 and tcp6\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("tcp4", s) == 0) {
            ls.config.tcp4 = positive;
            // fprintf(stderr, "%s tcp4\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("tcp6", s) == 0) {
            ls.config.tcp6 = positive;
            // fprintf(stderr, "%s tcp6\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("udp", s) == 0) {
            ls.config.udp4 = ls.config.udp6 = positive;
            // fprintf(stderr, "%s udp4 and udp6\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("udp4", s) == 0) {
            ls.config.udp4 = positive;
            // fprintf(stderr, "%s udp4\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("udp6", s) == 0) {
            ls.config.udp6 = positive;
            // fprintf(stderr, "%s udp6\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("ipv4", s) == 0) {
            ls.config.tcp4 = ls.config.udp4 = positive;
            // fprintf(stderr, "%s udp4 and tcp4\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("ipv6", s) == 0) {
            ls.config.tcp6 = ls.config.udp6 = positive;
            // fprintf(stderr, "%s udp6 and tcp6\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("listening", s) == 0) {
            ls.config.listening = positive;
            // fprintf(stderr, "%s listening\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("local", s) == 0) {
            ls.config.local = positive;
            // fprintf(stderr, "%s local\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("inbound", s) == 0) {
            ls.config.inbound = positive;
            // fprintf(stderr, "%s inbound\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("outbound", s) == 0) {
            ls.config.outbound = positive;
            // fprintf(stderr, "%s outbound\n", positive ? "enabling" : "disabling");
        }
        else if (strcmp("namespaces", s) == 0 || strcmp("ns", s) == 0) {
            ls.config.namespaces = positive;
            // fprintf(stderr, "%s namespaces\n", positive ? "enabling" : "disabling");
        }
        else {
            fprintf(stderr, "Unknown parameter %s\n", s);
            exit(1);
        }
    }

    local_sockets_process(&ls);

    return 0;
}
