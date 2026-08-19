<!--
Title: "Socket"
custom_edit_url: https://github.com/netdata/netdata/edit/master/src/libnetdata/socket/README.md
sidebar_label: "Socket"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Developers/libnetdata"
-->

<!-- markdownlint-disable MD043 -->

# Socket

The libnetdata socket utilities provide portable building blocks for outgoing connections, configured listeners, event polling,
access checks, peer inspection, and optional TLS transport. Include `libnetdata/libnetdata.h` to use these APIs; it exports the
public socket headers with the rest of libnetdata.

## API layers

| Header | Responsibility |
|---|---|
| [`socket.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/socket/socket.h) | File-descriptor checks, socket flags, buffer sizing, timed sends, accepted connections, access-list checks, and reverse hostname lookup. |
| [`connect-to.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/socket/connect-to.h) | Parse connection definitions and connect to one IPv4, IPv6, or Unix-socket destination, or try a list of destinations. |
| [`nd-sock.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/socket/nd-sock.h) | Own a client socket, report a typed `ND_SOCK_ERROR`, use plain or TLS I/O, apply send/receive timeouts, and close resources. |
| [`listen-sockets.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/socket/listen-sockets.h) | Build and close a bounded collection of TCP, UDP, IPv4, IPv6, or Unix listening sockets from an ini configuration section. |
| [`nd-poll.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/socket/nd-poll.h) | Register file descriptors and expose normalized read, write, error, hangup, invalid, timeout, and poll-failure events across platform polling backends. |
| [`poll-events.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/socket/poll-events.h) | Run the callback-based server loop used with `LISTEN_SOCKETS`, including connection, request, idle, and timer handling. |
| [`security.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/socket/security.h) | Initialize TLS contexts and perform client or nonblocking server handshakes and TLS I/O. |
| [`socket-peers.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/socket/socket-peers.h) | Read the local and remote IP address and port for a connected socket. |

## Connect to a destination

`connect_to_this()` accepts a connection definition in this form:

```text
[PROTOCOL:]HOST[%INTERFACE][:PORT]
```

`PROTOCOL` is `tcp` or `udp`. Enclose an IPv6 address in brackets; the optional interface applies to IPv6 scope. A definition
beginning with `unix:` or `/` selects a Unix-domain socket. When no port is present, the caller's default port is used.

Use `connect_to_one_of()` for a comma- or whitespace-separated destination list. It tries entries in order until one connects
and can return the selected entry and reconnection count. Use `connect_to_definition_get_service()` when only the effective
service or port needs to be resolved.

For a client that may use TLS, initialize an `ND_SOCK` with `ND_SOCK_INIT()` or `nd_sock_init()`, connect with
`nd_sock_connect_to_this()`, and inspect `error` when it fails. `nd_sock_send_timeout()` and `nd_sock_recv_timeout()` wait for
readiness and distinguish timeout, cancellation, polling, TLS, and connection errors. Finish with `nd_sock_close()` or declare
the value with `CLEAN_ND_SOCK` for scoped cleanup.

## Listen and poll

Populate a `LISTEN_SOCKETS` with its ini configuration, section, default bind definition, port, and backlog, then call
`listen_sockets_setup()`. The resulting arrays record every opened descriptor, its address family, socket type, display name,
and HTTP access flags. Always release them with `listen_sockets_close()`.

Use `nd_poll_create()`, `nd_poll_add()`, `nd_poll_upd()`, and `nd_poll_del()` to manage a portable poll set. The data pointer
registered with `nd_poll_add()` must be non-null and remain unchanged for that registration. `nd_poll_wait()` returns `1` with
one event, `0` for a timeout, and `-1` for a polling failure. Destroy the set with `nd_poll_destroy()`.

`poll_events()` is the higher-level listener loop. It accepts add, delete, receive, send, and timer callbacks and applies the
configured request timeout, idle timeout, access list, and maximum client count.
