# IPMI command adapter

This adapter uses the published `bougou/go-ipmi` revision `5691311a9607f327ff5229bb191e3567937d1ed1`. It reads the SDR
repository, sensor readings, and optionally the System Event Log (SEL). It does not write BMC configuration or the SEL.
Local OpenIPMI uses `NewOpenClient` and ordinary process/device permissions. LAN/LAN+ uses `NewClient` with the
configured address, credentials and privilege. One Reader serializes commands for one connection; the SDK serializes
remote exchanges with its automatic keepalive. SDK UDP retries are disabled.

## Discovery and missing data

Full and compact SDRs are decoded without the high-level `GetSensors` path. Numeric and alphabetic compact sharing
expands sensor numbers and names. The adapter addresses BMC-owned sensors on channel zero, including their LUN through
per-command context. Other owners/channels, invalid sharing, and unsupported Unicode ID strings remain unknown with an
aggregate warning; they are never queried as if owned by the default BMC. ASCII/Latin-1, BCD-plus, and packed 6-bit ID
strings use the SDR encoding.

The SDR repository count and modification dates are checked each collection. Records are cached between changes to avoid
rereading every SDR for each sample. A five-minute refresh catches firmware that leaves sentinel or unchanged dates;
metadata changes can therefore take up to five minutes to appear on such BMCs. Discovery rejects a changed repository
generation, malformed record lengths, and a chain that continues past the repository record count (which includes
cycles). As in FreeIPMI, a chain that ends before the record count is accepted, because some BMCs report a count that
does not match their records. Failed discovery is retried on the next collection. SDR reads follow FreeIPMI: a
whole-record read that fails with any completion code, or returns fewer bytes than the record announces, falls back to
partial reads. Partial reads use repository reservations when supported, reserve and retry up to four times when the BMC
reports the reservation cancelled or invalid, start with 16-byte chunks and shrink them by 4 bytes down to the header
size when the BMC refuses a size, continue after a shorter-than-requested response, and account for the 260-byte maximum
record with a one-byte offset. Transport failures are not retried; they fail the collection.

A disabled scanner or unavailable reading produces unknown state and no numeric sample. A missing status byte produces
unknown state even when the independent numeric reading is valid. Completion-code errors for individual sensors leave
healthy sensors collectable. Inventory failures, other sensor command failures and caller cancellation fail collection
and close the connection. A response decoding error is fatal even when the SDK wraps it in a `ResponseError` with
completion code zero. Closing a local device releases its descriptor without sending an IPMI command. Warnings are
counts in fixed categories, never an unbounded list of records.

Supported numeric units are Celsius, Fahrenheit, volts, amps, RPM, watts, and unitless percentages. Other unit
combinations retain status but omit numeric samples. Signed readings and standard linearizations are supported.
Nonlinear code `0x70` fetches conversion factors for that exact raw reading; OEM nonlinear codes, failed factor reads,
NaN, and infinity produce gaps. The SDK performs standard linearizations, including fractional EXP10 exponents.

Thresholds of threshold sensors with numeric values are read when the inventory is replaced, so every snapshot
carries them without extra commands. They use the reading's conversion; non-linear code `0x70` sensors have none, as
in FreeIPMI. SDR threshold access `00b` sends no command. Access `11b` (fixed, unreadable) uses the SDR values and its
readable threshold mask (Table 43-1 byte 19); the SDK parses the readable and settable masks swapped, so the adapter
reads its `Settable` flags. Other sensors use Get Sensor Thresholds and its readable mask. FreeIPMI's SDR fallback after
a refused Get Sensor Thresholds is not ported, because the SDR can hold values the BMC no longer uses. A
completion-code error leaves that sensor without thresholds; any other error stops threshold reads until the next
inventory refresh. Neither fails collection; a cancellation still does. Every collection reports the inventory's count
of unreadable thresholds, so the rate-limited warning log is not limited to refresh collections.

Keys use the C collector's healthy-reading identity: `i<record>_n<number>_t<legacy-reading-type>_u<legacy-unit>_<name>`.
They intentionally remain stable when a reading becomes unavailable: the C library changes reading/unit enums on that
transition. Expanded shared sensors and correctly decoded non-ASCII names can have different identities from the C
collector's default shared-sensor/name handling. Type labels follow the C collector; components use its ordered
case-insensitive name patterns, Other fallback, and fixed sensor-type overrides. This is not a complete FreeIPMI
compatibility layer.

## System Event Log

Each collection with SEL enabled sends `Get SEL Info`, and a second one when it completes a read (see below). The entry
count of the latest answer is the `ipmi.sel` sample; a failed first request omits the sample, with no previous value or
zero substitute. `Snapshot.SEL` appears once the BMC first answers and then carries
the cached records, the latest entry count, free bytes and overflow flag, and the clock information below. Any SEL
command failure degrades only SEL data: a failed `Get SEL Info` or record read keeps the earlier records and marks the
read failed, except that a record vanishing during a walk resets the cache (see below); a failed clock read keeps the
earlier clock values. Records resolve sensor names against the inventory again whenever it is replaced, so a renamed
sensor renames its records.

Records are cached per Reader, across collections and reconnects:

- The first read walks the chain from record 0000h with `Get SEL Entry`, oldest first. Each collection starts no new
  read once min(2 s, update interval / 4) has passed and resumes where it stopped; a walk reads at least one record. At
  about 8 ms per record over local KCS, a full read of a large SEL would otherwise delay sensor collection by seconds.
- Once the newest record has been read, an unchanged entry count and addition time send no `Get SEL Entry`. After a
  change, reading always resumes from the newest cached record, read again for its next-record ID, also when a read
  spanning several collections is still in progress: on a full log a BMC can give a new record the ID of an
  overwritten one, so a next-record ID saved earlier could lead past the records logged since. A cache that had
  reached the newest record first reads the BMC's newest record (FFFFh) and reads nothing more when they match.
- A changed erase time, or a newest cached record that is gone or changed, means the SEL was cleared or rewritten:
  the cache is reset and the log is read again. A record that disappears during a walk fails that read and restarts
  the log from 0000h.
- A read that added records and reached the newest record is complete only once verified: neither the end of the
  chain nor the entry count proves the cache holds the log, because a read that follows a record ID the BMC gave to a
  newer record skips the records in between. The BMC frees an ID in two ways, and the verification covers both:
  rollover overwrites the oldest records, so the oldest record the BMC holds (0000h) must be cached and older cached
  records are dropped; a deletion changes the erase time, so a second `Get SEL Info` must show the erase time
  unchanged. Otherwise the log is read again. A read still in progress also drops overwritten records once the cache
  holds more records than the entry count, so a log changing faster than it is read cannot grow the cache past its
  size. A cached record ID reported with new content was reused and replaces the old record. Reaching a cached record
  with unchanged content means the chain loops back and ends the walk.
- The Reader relies on IPMI 2.0 behavior: records leave the log only by rollover, which overwrites the oldest records,
  or by deletion or clear, which sets the erase time; and the chain lists records oldest first. The erase time counts
  seconds, so a deletion in the same second as the erase time the Reader last saw goes unnoticed: the cache keeps the
  deleted record until it would roll over, and if the BMC also gives its ID to a new record during a read, records
  can be skipped. A BMC that deletes records without setting the erase time has the same effect at any time.
- Like FreeIPMI, `Reserve SEL` is sent only after a 0xC5 completion code, and a read is retried with a new reservation
  up to four times. Whole 16-byte records are read; a shorter record fails the read.

Published record slices only grow at the end, lose a prefix or are reallocated, so earlier snapshots never change.

`SEL.New` holds counts of newly observed records by event type and default FreeIPMI state. After the first verified
read, event accounting is independent of the cached log's completeness:

- Before the first verified read, records are history and no event-accounting state is retained. If sustained rollover
  prevents that read from completing, no new-event counts are reported. A restart never reports that history as new.
  The first verified cache seeds accounting, including when the log is empty.
- Afterwards, every successful `Get SEL Entry` observation is accounted before cache mutation. One latest 16-byte
  occupant is retained per 16-bit record ID, so retention is bounded by the protocol ID space even if rescans never
  complete. Equal content at the same ID is a reread; different content replaces the prior occupant and counts once.
  An observed A -> B -> A transition counts the last A as a replacement. Identical content with no distinguishable
  intervening observation cannot establish a new occupant.
- Resets retain accounting. FIFO trim retires only the exact overwritten occupant; it cannot forget a newer occupant
  observed by a boundary probe. Verified completion drops IDs absent from the cache and preserves newer observations
  of retained IDs. The Function's mirror and its `Loaded` flag keep their existing verification rules.
- Once the baseline exists, a successful collection reports observed events even during a rescan. `NewKnown` is false
  before that baseline and on a failed SEL read. Cancellation fails the collection. Unpublished events are eight fixed
  counters, retained across failures and copied into the next eligible snapshot; returning that snapshot clears them.
  The backlog's duration therefore cannot grow event storage. Classification uses immutable event bytes, independently
  of the SDR inventory used for display names.

`SELRecord.Threshold` marks threshold events (event/reading type 01h), which the sensor state already reflects; every
other record is discrete.

`Get SEL Time UTC Offset` and `Get SEL Time` are read at most every five minutes, also when record reads fail. SEL time
is UTC plus the offset, so `SEL.Time` subtracts it. A completion-code failure, 07FFh or a value outside -1440..1440
minutes means no offset: times are then the BMC clock as UTC, and `ClockSkew` (BMC clock minus Agent clock) lets the
Function show the difference instead of guessing an offset. Pre-init timestamps (below 20000000h, seconds since BMC
initialization), FFFFFFFFh and non-timestamped OEM records have no date.

Records are decoded when read. Sensor names come from the SDR inventory by generator owner ID, LUN and sensor number.
Without an SDR record, the name is the generator class and sensor number, for example `BIOS #34`: software IDs
01h-1Fh BIOS, 21h-3Fh SMI Handler, 41h-5Fh System Management Software, 61h-7Fh OEM Software, 81h-8Dh Remote Console,
8Fh Terminal Mode Remote Console, other software IDs Software; IPMB address 20h is the BMC and others are controllers.
Event text comes from go-ipmi; threshold events whose event data carries the trigger reading or threshold append them,
converted with the sensor's SDR factors. OEM records are shown undecoded. Some firmware logs events with a reserved
record type in the standard range (00h-BFh) and the system event layout, for example BIOS power-off events as type 04h.
They are decoded for display, keep the unknown state of the FreeIPMI default policy, and name their record type in the
event text.

## Cancellation and cleanup

The Reader applies the configured timeout to the whole connection setup and to each command. The SDK observes caller
cancellation during local response waits and remote UDP exchanges. The SDK owns and joins its remote keepalive on close.
Local kernel ioctl execution itself is not interruptible by a Go context.

Remote Close uses one detached two-second context budget so a canceled or expired collection can still send Close
Session. The same path handles failed setup, failed collection, Check and explicit Cleanup. A lost Close Session
response can leave the BMC's session until its own expiry, but the client releases its local socket and goroutine.
The budget covers SDK shutdown after acquiring the Reader mutex, not time waiting for an ongoing Reader operation.
Closing a local device sends no command and needs no detached cleanup budget.

## State policy and source ownership

`states.go` ports all 76 discrete event-type/sensor-type dispatch combinations and the threshold policy from [FreeIPMI
1.6.17](https://ftp.gnu.org/gnu/freeipmi/freeipmi-1.6.17.tar.gz):

- `libfreeipmi/interpret/ipmi-interpret-config-sensor.c`: default state tables.
- `libfreeipmi/interpret/ipmi-interpret.c`: dispatch, aggregation, unknown offsets.
- `libipmimonitoring/ipmi_monitoring_sensor_reading.c`: units and missing readings.

`sel_states.go` ports the default SEL interpretation of the same release: all 82 tables with their 474 assertion and
deassertion states (`libfreeipmi/interpret/ipmi-interpret-config-sel.c`) and the dispatch of `ipmi_interpret_sel` in
`ipmi-interpret.c`. Only system event records (type 02h) are interpreted; OEM records, OEM and reserved event types,
pairs without a table and offsets past a table's end are unknown. FreeIPMI applies its Intel Node Manager and Supermicro
OEM tables, and treats invalid record types as system events, only with non-default flags. One table initializes only
the assertion state; its deassertion state is the zero value, nominal (`IPMI_INTERPRET_STATE_NOMINAL` is 0x00).
`testdata/freeipmi-default-sel-states.csv` lists every entry with its FreeIPMI option name, mechanically extracted from
those two files with each table mapped to its event/reading type and sensor type through the dispatch.

The source tables are GPL-3.0-or-later, copyright FreeIPMI Core Team (2003–2015), Lawrence Livermore National Security,
LLC (2007–2015), and The Regents of the University of California (2006–2007), written by Albert Chu at Lawrence
Livermore National Laboratory. Notices are retained in `states.go` and `sel_states.go`; the source-derived
`testdata/freeipmi-default-states.csv` and `testdata/freeipmi-default-sel-states.csv` carry the same license. The
repository LICENSE supplies its terms. Data and software are supplied without warranty.

FreeIPMI defaults lower/upper **non-critical** thresholds to **nominal**, while critical and nonrecoverable thresholds
are critical. Supported discrete states use their worst mapped severity. Unknown asserted offsets override recognized
severity; bit 15 is ignored. Unsupported event/sensor pairs are unknown even with no asserted bits. OEM interpretation,
local FreeIPMI configuration overrides, bridged satellites, and device-only SDR repositories are not implemented.

Tests exercise actual command-response decoding through a narrow fake transport, Reader collection/recovery/refresh, the
complete source-derived discrete and SEL fixtures, threshold flags, missing bytes, ownership, sharing, unit/sign
conversion, nonlinear factors, and fractional EXP10 conversion. SEL tests cover the read budget and resume, appends,
clears, rewrites, rollover and ID reuse, looping chains, reservations, UTC offsets, clock refresh, undated and OEM
records, generator names, failures and which records are new, including a replay of a real BMC's log
(`testdata/sel-replay.hex`) with synthetic critical discrete records. A randomized test (`sel_model_test.go`) mixes log
changes, rollover with reused IDs and failures, and checks against the fake BMC's log that the cache converges and that
every new record is reported exactly once. Real UDP tests use the upstream reference BMC with
a User account for both LAN and LAN+: Check cleanup, sensor and SEL collection, session reuse, malformed responses,
cancellation and failed setup recovery. These fixtures do not establish live-hardware parity for any particular vendor.

Threshold tests cover live and SDR readable masks, fixed thresholds, completion-code and other failures, cancellation,
the reading's conversion, and the refresh with the inventory.
