# IPMI command adapter

This experimental adapter uses the published `bougou/go-ipmi` revision `1462645b281c34c91d1a7c78298e46a54479e9e5`. It
reads the SDR repository, sensor readings, and optionally one SEL entry-count response. It does not write BMC
configuration. The only transport is Linux OpenIPMI using ordinary process/device permissions. The adapter accepts a
device number and command timeout, constructs `NewOpenClient`, and serializes its calls. LAN/LAN+ are deferred until
upstream session concurrency fixes are available; the pinned SDK's automatic keepalive can race between sequence capture
and request construction.

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
and close the connection. Closing the device closes its descriptor with the caller's context; it sends no IPMI command
and does not depend on the configured command timeout. Optional SEL failures omit its sample; no previous value or zero
substitute is returned. Each requested SEL collection performs a real, single `Get SEL Info` command rather than
rereading the log. Warnings are counts in four fixed categories, never an unbounded list of records.

Supported numeric units are Celsius, Fahrenheit, volts, amps, RPM, watts, and unitless percentages. Other unit
combinations retain status but omit numeric samples. Signed readings and standard linearizations are supported.
Nonlinear code `0x70` fetches conversion factors for that exact raw reading; OEM nonlinear codes, failed factor reads,
NaN, and infinity produce gaps. A narrow EXP10 workaround computes `10^x` after linear conversion because the pinned SDK
truncates fractional exponents. Remove it when a published dependency contains the upstream fix.

Keys use the C collector's healthy-reading identity: `i<record>_n<number>_t<legacy-reading-type>_u<legacy-unit>_<name>`.
They intentionally remain stable when a reading becomes unavailable: the C library changes reading/unit enums on that
transition. Expanded shared sensors and correctly decoded non-ASCII names can have different identities from the C
collector's default shared-sensor/name handling. Type labels follow the C collector; components use its ordered
case-insensitive name patterns, Other fallback, and fixed sensor-type overrides. This is not a complete FreeIPMI
compatibility layer.

## Local receive deadline limitation

The pinned Linux SDK does not observe context cancellation during its blocking receive. The adapter checks cancellation
around every command and passes `min(configured timeout, remaining caller budget)` as the SDK receive timeout. This
temporary SDK setting is restored after each serialized command. It improves deadline fidelity without changing the
public timeout option or replacing the transport.

Each receive is bounded by that timeout, but cancellation after receive begins cannot interrupt it. The SDK starts its
relative receive deadline after command send overhead, so the caller's deadline is not an exact wall-clock guarantee.

## State policy and source ownership

`states.go` ports all 76 discrete event-type/sensor-type dispatch combinations and the threshold policy from [FreeIPMI
1.6.17](https://ftp.gnu.org/gnu/freeipmi/freeipmi-1.6.17.tar.gz):

- `libfreeipmi/interpret/ipmi-interpret-config-sensor.c`: default state tables.
- `libfreeipmi/interpret/ipmi-interpret.c`: dispatch, aggregation, unknown offsets.
- `libipmimonitoring/ipmi_monitoring_sensor_reading.c`: units and missing readings.

The source tables are GPL-3.0-or-later, copyright FreeIPMI Core Team (2003–2015), Lawrence Livermore National Security,
LLC (2007–2015), and The Regents of the University of California (2006–2007), written by Albert Chu at Lawrence
Livermore National Laboratory. Notices are retained in `states.go`; the source-derived
`testdata/freeipmi-default-states.csv` carries the same license. The repository LICENSE supplies its terms. Data and
software are supplied without warranty.

FreeIPMI defaults lower/upper **non-critical** thresholds to **nominal**, while critical and nonrecoverable thresholds
are critical. Supported discrete states use their worst mapped severity. Unknown asserted offsets override recognized
severity; bit 15 is ignored. Unsupported event/sensor pairs are unknown even with no asserted bits. OEM interpretation,
local FreeIPMI configuration overrides, bridged satellites, and device-only SDR repositories are not implemented.

Tests exercise actual command-response decoding through a narrow fake transport, Reader collection/recovery/refresh, the
complete source-derived discrete fixture, threshold flags, missing bytes, ownership, sharing, unit/sign conversion,
nonlinear factors, and the pinned EXP10 workaround. They do not establish live-hardware parity for any particular
vendor; separate live OpenIPMI evidence is required.
