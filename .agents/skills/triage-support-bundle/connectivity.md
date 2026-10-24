# Streaming, Cloud, and the dashboard

Three independent failure surfaces that reporters all describe as "offline" or "not connecting".
Establish which one is meant before choosing evidence: a node can stream perfectly and be offline in
Cloud, or be connected to Cloud with no dashboard reachable from the reporter's browser.

Owners: `src/streaming/README.md#verifying-successful-connections` and
`src/streaming/README.md#troubleshooting` for streaming, including the verbatim log lines it
documents; `src/claim/README.md#connection-troubleshooting` and
`src/claim/README.md#common-issues` for claiming and the cloud connection;
`src/libnetdata/log/README.md#message-ids` for isolating connection records in the logs.

## Streaming

**The parent names its own reason.** The receiver emits an explicit rejection reason for every
refused connection, and those reasons discriminate causes nothing else can separate - an API key that
is actually a machine identifier and the reverse, a key not enabled versus not permitted from that
address, a host already served by another receiver, a rate limit, and a machine identifier arriving
with a different hostname than before. The full set lives in the receiver source,
`src/streaming/stream-receiver-connection.c`; the operator-facing subset with example lines is in
`src/streaming/README.md#troubleshooting`. Read the reason before theorising.

Order of checks:

1. **Which end is this bundle?** A child bundle shows what was sent and whether the attempt was made.
   A parent bundle shows the decision. Most streaming questions need both - say so early rather than
   guessing from one.
2. **Did the connection reach the parent at all?** The parent's access log distinguishes "never
   arrived" (network, address, firewall) from "arrived and was refused" (configuration, identity).
   This single split removes most wrong theories.
3. **The rejection reason**, if it arrived. That is usually the whole diagnosis.
4. **The stream configuration on both ends.** The streaming API key is kept verbatim in the collected
   stream configuration precisely so a child's key can be compared against a parent's - that
   comparison is the point of the exception.
5. **Clock.** Drift on a child silently breaks streaming and cloud authentication, and is captured on
   both platforms.
6. **Identity.** A machine identifier arriving under a different hostname is the cloned-VM signature;
   see `./false-signals.md`.

Recurring causes, as patterns: stream configuration mismatch between the ends; a byte-order mark or
an inline comment breaking the parsing of a key section; children claimed directly to Cloud instead
of streaming through the parent; identity collisions from cloned images; parent capacity; and stale
child entries. Note that the two ends may legitimately sample at different rates - that is never
itself a streaming fault.

## Cloud and claiming

Order of checks:

1. **Claim state.** The bundle carries the claim identifier only; the token and private key are never
   collected. A claim directory that does not persist across restarts is a recurring root cause, and
   the state listing shows it.
2. **The agent's view of the connection**, from the runtime captures. This is the agent's opinion,
   not Cloud's.
3. **The network path.** The Cloud probe validates the certificate and **retains** the real proxy
   configuration, so it represents the installation's actual path - unlike the local API reads, which
   deliberately clear proxy variables. A certificate failure and a connection timeout are different
   findings.
4. **The environment the service sees**, which differs from the reporter's shell. Proxy variables set
   in a login shell do not reach a system service.
5. **Clock**, again.

Recurring causes, as patterns: stale node or claim state; certificate chain validation failing on
older base images or behind inspection proxies; claim state not persisting across a reinstall;
resolver failure inside a container rather than on the host; and claiming attempted before the daemon
was running.

**The bundle only sees the agent's half.** "The agent says connected, Cloud says offline" is not
resolvable from a bundle - that needs `query-netdata-cloud`. See `./evidence-limits.md`.

## Dashboard reachability

Order of checks:

1. **The socket inventory** settles bind address versus firewall immediately, and it covers the whole
   netdata process tree rather than a host-wide listing. An agent bound to the loopback address is
   the single most common cause, and it is indistinguishable from a firewall problem without this
   file.
2. **The effective web configuration** - the bind setting and TLS material - rather than the on-disk
   file.
3. **The access log** answers whether the request arrived at all. If it did not, nothing in the agent
   is at fault and the problem is in front of it.
4. **Permissions on the TLS directory**, which the permissions area reports without ever collecting
   key material.

**Everything in front of the agent is invisible.** No reverse-proxy configuration, no proxy access
log, no browser view, no certificate chain as the client sees it. A misconfigured proxy leaves the
bundle looking perfectly healthy. Say that plainly rather than reporting "the agent is fine".

## Traps

- A reporter saying "offline" almost never specifies which of the three surfaces they mean. Ask.
- Reinstalling does not change node identity, so it does not fix identity collisions - see
  `./false-signals.md`.
- The claim identifier and the machine identifier are different things and are routinely conflated.
- A missing runtime area means the API was unreachable, which has three possible causes and is not
  by itself evidence that the agent was down.
