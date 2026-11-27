// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import "strings"

// smbServerCounters are the counters of the Server counter set, which
// describes the host's SMB file server (the Server service) as a whole. The
// native Windows Agent does not chart it. The current sessions and open files
// are point values; every other counter counts since the service started.
var smbServerCounters = [...]rawCounter{
	{property: "BytesReceivedPersec", metric: "smb_server_received_bytes", total: true},
	{property: "BytesTransmittedPersec", metric: "smb_server_sent_bytes", total: true},
	{property: "ServerSessions", metric: "smb_server_sessions"},
	{property: "SessionsLoggedOff", metric: "smb_server_sessions_logged_off", total: true},
	{property: "SessionsTimedOut", metric: "smb_server_sessions_timed_out", total: true},
	{property: "SessionsErroredOut", metric: "smb_server_sessions_errored_out", total: true},
	{property: "SessionsForcedOff", metric: "smb_server_sessions_forced_off", total: true},
	// Logon/sec counts every logon attempt, failed ones included; its raw
	// value equals Logon Total.
	{property: "LogonPersec", metric: "smb_server_logons", total: true},
	{property: "ErrorsLogon", metric: "smb_server_logon_errors", total: true},
	{property: "ErrorsAccessPermissions", metric: "smb_server_access_permission_errors", total: true},
	{property: "ErrorsGrantedAccess", metric: "smb_server_granted_access_errors", total: true},
	{property: "FilesOpen", metric: "smb_server_open_files"},
	{property: "FilesOpenedTotal", metric: "smb_server_files_opened", total: true},
	{property: "ErrorsSystem", metric: "smb_server_system_errors", total: true},
	{property: "WorkItemShortages", metric: "smb_server_work_item_shortages", total: true},
	{property: "BlockingRequestsRejected", metric: "smb_server_blocking_requests_rejected", total: true},
	{property: "PoolPagedFailures", metric: "smb_server_paged_pool_failures", total: true},
	{property: "PoolNonpagedFailures", metric: "smb_server_nonpaged_pool_failures", total: true},
}

// smbShareCounters are the counters of the SMB Server Shares counter set that
// the native Windows Agent charts per share.
var smbShareCounters = [...]rawCounter{
	{property: "CurrentOpenFileCount", metric: "smb_share_open_files"},
	{property: "TreeConnectCount", metric: "smb_share_tree_connects"},
	{property: "ReadRequestsPersec", metric: "smb_share_read_requests", total: true},
	{property: "WriteRequestsPersec", metric: "smb_share_write_requests", total: true},
	{property: "MetadataRequestsPersec", metric: "smb_share_metadata_requests", total: true},
	{property: "FilesOpenedPersec", metric: "smb_share_files_opened", total: true},
	{property: "ReceivedBytesPersec", metric: "smb_share_received_bytes", total: true},
	{property: "SentBytesPersec", metric: "smb_share_sent_bytes", total: true},
}

var (
	querySMBServer = counterQuery("Win32_PerfRawData_PerfNet_Server", smbServerCounters[:])
	// SMB Server Shares is a manifest-based counter set, so WMI names its
	// class after the Counters provider.
	querySMBShares = counterQuery("Win32_PerfRawData_Counters_SMBServerShares", smbShareCounters[:], "Name")
)

// smbShare holds one share's counter values. Its name is the counter set's
// instance name, such as \\*\C$.
type smbShare struct {
	name   string
	values [len(smbShareCounters)]sample
}

func (o *observations) readSMBServer(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	return readCounters(rows[0], smbServerCounters[:], o.smbServer[:])
}

// readSMBShares keeps the shares that report a value. Like the native Windows
// Agent, it skips the counter set's _Total and unnamed instances. A host
// without shares answers with no instance or with _Total only.
func (o *observations) readSMBShares(rows []map[string]any) bool {
	var shares int
	for _, row := range rows {
		name, _ := row["name"].(string)
		if name == "" || strings.EqualFold(name, "_Total") {
			continue
		}
		shares++
		share := smbShare{
			name: name,
		}
		if readCounters(row, smbShareCounters[:], share.values[:]) {
			o.smbShares = append(o.smbShares, share)
		}
	}
	return shares == 0 || len(o.smbShares) > 0
}
