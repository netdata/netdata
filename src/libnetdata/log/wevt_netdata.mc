;// SPDX-License-Identifier: GPL-3.0-or-later

SeverityNames=(Success=0x0:STATUS_SEVERITY_SUCCESS
               Informational=0x1:STATUS_SEVERITY_INFORMATIONAL
               Warning=0x2:STATUS_SEVERITY_WARNING
               Error=0x3:STATUS_SEVERITY_ERROR
              )

FacilityNames=(System=0x0:FACILITY_SYSTEM
               Runtime=0x2:FACILITY_RUNTIME
               Stubs=0x3:FACILITY_STUBS
               Io=0x4:FACILITY_IO_ERROR_CODE
              )

LanguageNames=(English=0x409:MSG00409)

MessageIdTypedef=WORD

MessageId=0x1
SymbolicName=ND_PROVIDER_NAME
Language=English
Netdata
.

MessageId=0x2
SymbolicName=NETDATA_MSG_VERSION
Language=English
1.0
.

MessageId=0x3
SymbolicName=ND_GENERIC_LOG_MESSAGE
Language=English
%64
.

MessageId=0x4
SymbolicName=ND_ACCESS_EVENT_MESSAGE
Language=English
Transaction %36, method: %33, path: %63

    Source IP     : %24, Forwarded-For: %27
    User          : %21, role: %22, permissions: %23
    Timings (usec): prep %39, sent %40, total %41
    Response Size : sent %37, uncompressed %38
    Response Code : %34
.

MessageId=0x5
SymbolicName=ND_HEALTH_EVENT_MESSAGE
Language=English
Alert '%47' of instance '%16' on node '%15', transitioned from %57 to %56
.

;// -------------------------------------------------------------------------------------------------------------------
;// category

MessageId=0x20
SymbolicName=NETDATA_DAEMON_CATEGORY
Language=English
Netdata Daemon Log
.

MessageId=0x21
SymbolicName=NETDATA_COLLECTOR_CATEGORY
Language=English
Netdata Collector Log
.

MessageId=0x22
SymbolicName=NETDATA_ACCESS_CATEGORY
Language=English
Netdata Access Log
.

MessageId=0x23
SymbolicName=NETDATA_HEALTH_CATEGORY
Language=English
Netdata Alert Transition
.

MessageId=0x24
SymbolicName=NETDATA_ACLK_CATEGORY
Language=English
Netdata ACLK Log
.

;// -------------------------------------------------------------------------------------------------------------------
;// daemon.log

MessageId=0x1000
Severity=Informational
Facility=Application
SymbolicName=ND_EVENT_DAEMON_INFO
Language=English
%64
.

MessageId=0x1001
Severity=Warning
Facility=Application
SymbolicName=ND_EVENT_DAEMON_WARNING
Language=English
%64
.

MessageId=0x1002
Severity=Error
Facility=Application
SymbolicName=ND_EVENT_DAEMON_ERROR
Language=English
%64
.

;// -------------------------------------------------------------------------------------------------------------------
;// collector.log

MessageId=0x2000
Severity=Informational
Facility=Application
SymbolicName=ND_EVENT_COLLECTOR_INFO
Language=English
%64
.

MessageId=0x2001
Severity=Warning
Facility=Application
SymbolicName=ND_EVENT_COLLECTOR_WARNING
Language=English
%64
.

MessageId=0x2002
Severity=Error
Facility=Application
SymbolicName=ND_EVENT_COLLECTOR_ERROR
Language=English
%64
.

;// -------------------------------------------------------------------------------------------------------------------
;// access.log

MessageId=0x3000
Severity=Informational
Facility=Application
SymbolicName=ND_EVENT_ACCESS_INFO
Language=English
Transaction %36, method: %33, path: %63

    Source IP     : %24, Forwarded-For: %27
    User          : %21, role: %22, permissions: %23
    Timings (usec): prep %39, sent %40, total %41
    Response Size : sent %37, uncompressed %38
    Response Code : %34
.

MessageId=0x3001
Severity=Warning
Facility=Application
SymbolicName=ND_EVENT_ACCESS_WARNING
Language=English
Transaction %36, method: %33, path: %63

    Source IP     : %24, Forwarded-For: %27
    User          : %21, role: %22, permissions: %23
    Timings (usec): prep %39, sent %40, total %41
    Response Size : sent %37, uncompressed %38
    Response Code : %34
.

MessageId=0x3002
Severity=Error
Facility=Application
SymbolicName=ND_EVENT_ACCESS_ERROR
Language=English
Transaction %36, method: %33, path: %63

    Source IP     : %24, Forwarded-For: %27
    User          : %21, role: %22, permissions: %23
    Timings (usec): prep %39, sent %40, total %41
    Response Size : sent %37, uncompressed %38
    Response Code : %34
.

;// -------------------------------------------------------------------------------------------------------------------
;// health.log

MessageId=0x4000
Severity=Informational
Facility=Application
SymbolicName=ND_EVENT_HEALTH_INFO
Language=English
Alert '%47' of instance '%16' on node '%15', transitioned from %57 to %56
.

MessageId=0x4001
Severity=Warning
Facility=Application
SymbolicName=ND_EVENT_HEALTH_WARNING
Language=English
Alert '%47' of instance '%16' on node '%15', transitioned from %57 to %56
.

MessageId=0x4002
Severity=Error
Facility=Application
SymbolicName=ND_EVENT_HEALTH_ERROR
Language=English
Alert '%47' of instance '%16' on node '%15', transitioned from %57 to %56
.

;// -------------------------------------------------------------------------------------------------------------------
;// aclk.log

MessageId=0x5000
Severity=Informational
Facility=Application
SymbolicName=ND_EVENT_ACLK_INFO
Language=English
ACLK LOG: %64
.

MessageId=0x5001
Severity=Warning
Facility=Application
SymbolicName=ND_EVENT_ACLK_WARNING
Language=English
ACLK LOG: %64
.

MessageId=0x5002
Severity=Error
Facility=Application
SymbolicName=ND_EVENT_ACLK_ERROR
Language=English
ACLK LOG: %64
.
