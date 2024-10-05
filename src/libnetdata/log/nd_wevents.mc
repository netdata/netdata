;// SPDX-License-Identifier: GPL-3.0-or-later
;//
;// Required Programs:
;// mc       - a program in Windows SDK
;// rc       - a program in Windows SDK
;// link     - a program in Visual Studio
;// icacls   - a standard system program
;// wevtutil - a standard system program
;//
;// Compile with:
;// mc nd_wevents.mc
;//  -> generates nd_wevents.rc and MSG00001.bin
;//
;// rc nd_wevents.rc
;//  -> generates nd_wevents.res and nd_wevents.h
;//
;// link /dll /noentry /machine:X64 /out:nd_wevents.dll nd_wevents.res
;//  -> generates nd_wevents.dll
;//
;// To install:
;// copy nd_wevents.dll C:\Windows\System32
;//
;// Give access to Windows Events Log:
;// icacls "C:\Windows\System32\nd_wevents.dll" /grant "NT SERVICE\EventLog":R
;//
;// To register it:
;// wevtutil im NetdataEventManifest.xml
;//

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
