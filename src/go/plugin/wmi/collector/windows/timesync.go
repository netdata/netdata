// SPDX-License-Identifier: GPL-3.0-or-later

package windows

// timeServiceCounters are the Windows Time Service counter set's point values
// that describe how the host keeps its own clock synchronized: the size of its
// clock's offset from its time source, as the service computes it, and the
// round trip of its latest NTP request, both in microseconds, and the number of
// time servers that answer it. The native Windows Agent does not chart them.
var timeServiceCounters = [...]rawCounter{
	{property: "ComputedTimeOffset", metric: "time_service_offset_microseconds"},
	{property: "NTPRoundtripDelay", metric: "time_service_ntp_roundtrip_delay_microseconds"},
	{property: "NTPClientTimeSourceCount", metric: "time_service_ntp_sources"},
}

// The Windows Time Service counter set exists since Windows Server 2016. It is
// manifest-based, so WMI names its class after its provider.
var queryTimeService = counterQuery(
	"Win32_PerfRawData_MicrosoftWindowsW32TimePerf_WindowsTimeService",
	timeServiceCounters[:],
)

func (o *observations) readTimeService(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	return readCounters(rows[0], timeServiceCounters[:], o.timeService[:])
}
