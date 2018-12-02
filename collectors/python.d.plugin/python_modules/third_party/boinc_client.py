#!/usr/bin/env python
# -*- coding: utf-8 -*-
#
# client.py - Somewhat higher-level GUI_RPC API for BOINC core client
#
#    Copyright (C) 2013 Rodrigo Silva (MestreLion) <linux@rodrigosilva.com>
#    Copyright (C) 2017 Austin S. Hemmelgarn
#
# SPDX-License-Identifier: GPL-3.0

# Based on client/boinc_cmd.cpp

import hashlib
import socket
import sys
import time
from functools import total_ordering
from xml.etree import ElementTree

GUI_RPC_PASSWD_FILE = "/var/lib/boinc/gui_rpc_auth.cfg"

GUI_RPC_HOSTNAME    = None  # localhost
GUI_RPC_PORT        = 31416
GUI_RPC_TIMEOUT     = 1

class Rpc(object):
    ''' Class to perform GUI RPC calls to a BOINC core client.
        Usage in a context manager ('with' block) is recommended to ensure
        disconnect() is called. Using the same instance for all calls is also
        recommended so it reuses the same socket connection
        '''
    def __init__(self, hostname="", port=0, timeout=0, text_output=False):
        self.hostname = hostname
        self.port     = port
        self.timeout  = timeout
        self.sock = None
        self.text_output = text_output

    @property
    def sockargs(self):
        return (self.hostname, self.port, self.timeout)

    def __enter__(self): self.connect(*self.sockargs); return self
    def __exit__(self, *args): self.disconnect()

    def connect(self, hostname="", port=0, timeout=0):
        ''' Connect to (hostname, port) with timeout in seconds.
            Hostname defaults to None (localhost), and port to 31416
            Calling multiple times will disconnect previous connection (if any),
            and (re-)connect to host.
        '''
        if self.sock:
            self.disconnect()

        self.hostname = hostname or GUI_RPC_HOSTNAME
        self.port     = port     or GUI_RPC_PORT
        self.timeout  = timeout  or GUI_RPC_TIMEOUT

        self.sock = socket.create_connection(self.sockargs[0:2], self.sockargs[2])

    def disconnect(self):
        ''' Disconnect from host. Calling multiple times is OK (idempotent)
        '''
        if self.sock:
            self.sock.close()
            self.sock = None

    def call(self, request, text_output=None):
        ''' Do an RPC call. Pack and send the XML request and return the
            unpacked reply. request can be either plain XML text or a
            xml.etree.ElementTree.Element object. Return ElementTree.Element
            or XML text according to text_output flag.
            Will auto-connect if not connected.
        '''
        if text_output is None:
            text_output = self.text_output

        if not self.sock:
            self.connect(*self.sockargs)

        if not isinstance(request, ElementTree.Element):
            request = ElementTree.fromstring(request)

        # pack request
        end = '\003'
        if sys.version_info[0] < 3:
            req = "<boinc_gui_rpc_request>\n{0}\n</boinc_gui_rpc_request>\n{1}".format(ElementTree.tostring(request).replace(' />', '/>'), end)
        else:
            req = "<boinc_gui_rpc_request>\n{0}\n</boinc_gui_rpc_request>\n{1}".format(ElementTree.tostring(request, encoding='unicode').replace(' />', '/>'), end).encode()

        try:
            self.sock.sendall(req)
        except (socket.error, socket.herror, socket.gaierror, socket.timeout):
            raise

        req = ""
        while True:
            try:
                buf = self.sock.recv(8192)
                if not buf:
                    raise socket.error("No data from socket")
                if sys.version_info[0] >= 3:
                    buf = buf.decode()
            except socket.error:
                raise
            n = buf.find(end)
            if not n == -1: break
            req += buf
        req += buf[:n]

        # unpack reply (remove root tag, ie: first and last lines)
        req = '\n'.join(req.strip().rsplit('\n')[1:-1])

        if text_output:
            return req
        else:
            return ElementTree.fromstring(req)

def setattrs_from_xml(obj, xml, attrfuncdict={}):
    ''' Helper to set values for attributes of a class instance by mapping
        matching tags from a XML file.
        attrfuncdict is a dict of functions to customize value data type of
        each attribute. It falls back to simple int/float/bool/str detection
        based on values defined in __init__(). This would not be needed if
        Boinc used standard RPC protocol, which includes data type in XML.
    '''
    if not isinstance(xml, ElementTree.Element):
        xml = ElementTree.fromstring(xml)
    for e in list(xml):
        if hasattr(obj, e.tag):
            attr = getattr(obj, e.tag)
            attrfunc = attrfuncdict.get(e.tag, None)
            if attrfunc is None:
                if   isinstance(attr, bool):  attrfunc = parse_bool
                elif isinstance(attr, int):   attrfunc = parse_int
                elif isinstance(attr, float): attrfunc = parse_float
                elif isinstance(attr, str):   attrfunc = parse_str
                elif isinstance(attr, list):  attrfunc = parse_list
                else:                         attrfunc = lambda x: x
            setattr(obj, e.tag, attrfunc(e))
        else:
            pass
            #print "class missing attribute '%s': %r" % (e.tag, obj)
    return obj


def parse_bool(e):
    ''' Helper to convert ElementTree.Element.text to boolean.
        Treat '<foo/>' (and '<foo>[[:blank:]]</foo>') as True
        Treat '0' and 'false' as False
    '''
    if e.text is None:
        return True
    else:
        return bool(e.text) and not e.text.strip().lower() in ('0', 'false')


def parse_int(e):
    ''' Helper to convert ElementTree.Element.text to integer.
        Treat '<foo/>' (and '<foo></foo>') as 0
    '''
    # int(float()) allows casting to int a value expressed as float in XML
    return 0 if e.text is None else int(float(e.text.strip()))


def parse_float(e):
    ''' Helper to convert ElementTree.Element.text to float. '''
    return 0.0 if e.text is None else float(e.text.strip())


def parse_str(e):
    ''' Helper to convert ElementTree.Element.text to string. '''
    return "" if e.text is None else e.text.strip()


def parse_list(e):
    ''' Helper to convert  ElementTree.Element to list. For now, simply return
        the list of root element's children
    '''
    return list(e)


class Enum(object):
    UNKNOWN                =   -1  # Not in original API

    @classmethod
    def name(cls, value):
        ''' Quick-and-dirty fallback for getting the "name" of an enum item '''

        # value as string, if it matches an enum attribute.
        # Allows short usage as Enum.name("VALUE") besides Enum.name(Enum.VALUE)
        if hasattr(cls, str(value)):
            return cls.name(getattr(cls, value, None))

        # value not handled in subclass name()
        for k, v in cls.__dict__.items():
            if v == value:
                return k.lower().replace('_', ' ')

        # value not found
        return cls.name(Enum.UNKNOWN)


class CpuSched(Enum):
    ''' values of ACTIVE_TASK::scheduler_state and ACTIVE_TASK::next_scheduler_state
        "SCHEDULED" is synonymous with "executing" except when CPU throttling
        is in use.
    '''
    UNINITIALIZED          =    0
    PREEMPTED              =    1
    SCHEDULED              =    2


class ResultState(Enum):
    ''' Values of RESULT::state in client.
        THESE MUST BE IN NUMERICAL ORDER
        (because of the > comparison in RESULT::computing_done())
        see html/inc/common_defs.inc
    '''
    NEW                    =    0
        #// New result
    FILES_DOWNLOADING      =    1
        #// Input files for result (WU, app version) are being downloaded
    FILES_DOWNLOADED       =    2
        #// Files are downloaded, result can be (or is being) computed
    COMPUTE_ERROR          =    3
        #// computation failed; no file upload
    FILES_UPLOADING        =    4
        #// Output files for result are being uploaded
    FILES_UPLOADED         =    5
        #// Files are uploaded, notify scheduling server at some point
    ABORTED                =    6
        #// result was aborted
    UPLOAD_FAILED          =    7
        #// some output file permanent failure


class Process(Enum):
    ''' values of ACTIVE_TASK::task_state '''
    UNINITIALIZED          =    0
        #// process doesn't exist yet
    EXECUTING              =    1
        #// process is running, as far as we know
    SUSPENDED              =    9
        #// we've sent it a "suspend" message
    ABORT_PENDING          =    5
        #// process exceeded limits; send "abort" message, waiting to exit
    QUIT_PENDING           =    8
        #// we've sent it a "quit" message, waiting to exit
    COPY_PENDING           =   10
        #// waiting for async file copies to finish


class _Struct(object):
    ''' base helper class with common methods for all classes derived from
        BOINC's C++ structs
    '''
    @classmethod
    def parse(cls, xml):
        return setattrs_from_xml(cls(), xml)

    def __str__(self, indent=0):
        buf = '{0}{1}:\n'.format('\t' * indent, self.__class__.__name__)
        for attr in self.__dict__:
            value = getattr(self, attr)
            if isinstance(value, list):
                buf += '{0}\t{1} [\n'.format('\t' * indent, attr)
                for v in value: buf += '\t\t{0}\t\t,\n'.format(v)
                buf += '\t]\n'
            else:
                buf += '{0}\t{1}\t{2}\n'.format('\t' * indent,
                                         attr,
                                         value.__str__(indent+2)
                                            if isinstance(value, _Struct)
                                            else repr(value))
        return buf


@total_ordering
class VersionInfo(_Struct):
    def __init__(self, major=0, minor=0, release=0):
        self.major     = major
        self.minor     = minor
        self.release   = release

    @property
    def _tuple(self):
        return  (self.major, self.minor, self.release)

    def __eq__(self, other):
        return isinstance(other, self.__class__) and self._tuple == other._tuple

    def __ne__(self, other):
        return not self.__eq__(other)

    def __gt__(self, other):
        if not isinstance(other, self.__class__):
            return NotImplemented
        return self._tuple > other._tuple

    def __str__(self):
        return "{0}.{1}.{2}".format(self.major, self.minor, self.release)

    def __repr__(self):
        return "{0}{1}".format(self.__class__.__name__, self._tuple)


class Result(_Struct):
    ''' Also called "task" in some contexts '''
    def __init__(self):
        # Names and values follow lib/gui_rpc_client.h @ RESULT
        # Order too, except when grouping contradicts client/result.cpp
        # RESULT::write_gui(), then XML order is used.

        self.name                         = ""
        self.wu_name                      = ""
        self.version_num                  = 0
            #// identifies the app used
        self.plan_class                   = ""
        self.project_url                  = ""  # from PROJECT.master_url
        self.report_deadline              = 0.0 # seconds since epoch
        self.received_time                = 0.0 # seconds since epoch
            #// when we got this from server
        self.ready_to_report              = False
            #// we're ready to report this result to the server;
            #// either computation is done and all the files have been uploaded
            #// or there was an error
        self.got_server_ack               = False
            #// we've received the ack for this result from the server
        self.final_cpu_time               = 0.0
        self.final_elapsed_time           = 0.0
        self.state                        = ResultState.NEW
        self.estimated_cpu_time_remaining = 0.0
            #// actually, estimated elapsed time remaining
        self.exit_status                  = 0
            #// return value from the application
        self.suspended_via_gui            = False
        self.project_suspended_via_gui    = False
        self.edf_scheduled                = False
            #// temporary used to tell GUI that this result is deadline-scheduled
        self.coproc_missing               = False
            #// a coproc needed by this job is missing
            #// (e.g. because user removed their GPU board).
        self.scheduler_wait               = False
        self.scheduler_wait_reason        = ""
        self.network_wait                 = False
        self.resources                    = ""
            #// textual description of resources used

        #// the following defined if active
        # XML is generated in client/app.cpp ACTIVE_TASK::write_gui()
        self.active_task                  = False
        self.active_task_state            = Process.UNINITIALIZED
        self.app_version_num              = 0
        self.slot                         = -1
        self.pid                          = 0
        self.scheduler_state              = CpuSched.UNINITIALIZED
        self.checkpoint_cpu_time          = 0.0
        self.current_cpu_time             = 0.0
        self.fraction_done                = 0.0
        self.elapsed_time                 = 0.0
        self.swap_size                    = 0
        self.working_set_size_smoothed    = 0.0
        self.too_large                    = False
        self.needs_shmem                  = False
        self.graphics_exec_path           = ""
        self.web_graphics_url             = ""
        self.remote_desktop_addr          = ""
        self.slot_path                    = ""
            #// only present if graphics_exec_path is

        # The following are not in original API, but are present in RPC XML reply
        self.completed_time               = 0.0
            #// time when ready_to_report was set
        self.report_immediately           = False
        self.working_set_size             = 0
        self.page_fault_rate              = 0.0
            #// derived by higher-level code

        # The following are in API, but are NEVER in RPC XML reply. Go figure
        self.signal                       = 0

        self.app                          = None  # APP*
        self.wup                          = None  # WORKUNIT*
        self.project                      = None  # PROJECT*
        self.avp                          = None  # APP_VERSION*

    @classmethod
    def parse(cls, xml):
        if not isinstance(xml, ElementTree.Element):
            xml = ElementTree.fromstring(xml)

        # parse main XML
        result = super(Result, cls).parse(xml)

        # parse '<active_task>' children
        active_task = xml.find('active_task')
        if active_task is None:
            result.active_task = False  # already the default after __init__()
        else:
            result.active_task = True   # already the default after main parse
            result = setattrs_from_xml(result, active_task)

        #// if CPU time is nonzero but elapsed time is zero,
        #// we must be talking to an old client.
        #// Set elapsed = CPU
        #// (easier to deal with this here than in the manager)
        if result.current_cpu_time != 0 and result.elapsed_time == 0:
            result.elapsed_time = result.current_cpu_time

        if result.final_cpu_time != 0 and result.final_elapsed_time == 0:
            result.final_elapsed_time = result.final_cpu_time

        return result

    def __str__(self):
        buf = '{0}:\n'.format(self.__class__.__name__)
        for attr in self.__dict__:
            value = getattr(self, attr)
            if attr in ['received_time', 'report_deadline']:
                value = time.ctime(value)
            buf += '\t{0}\t{1}\n'.format(attr, value)
        return buf


class BoincClient(object):

    def __init__(self, host="", port=0, passwd=None):
        self.hostname   = host
        self.port       = port
        self.passwd     = passwd
        self.rpc        = Rpc(text_output=False)
        self.version    = None
        self.authorized = False

        # Informative, not authoritative. Records status of *last* RPC call,
        # but does not infer success about the *next* one.
        # Thus, it should be read *after* an RPC call, not prior to one
        self.connected = False

    def __enter__(self): self.connect(); return self
    def __exit__(self, *args): self.disconnect()

    def connect(self):
        try:
            self.rpc.connect(self.hostname, self.port)
            self.connected = True
        except socket.error:
            self.connected = False
            return
        self.authorized = self.authorize(self.passwd)
        self.version = self.exchange_versions()

    def disconnect(self):
        self.rpc.disconnect()

    def authorize(self, password):
        ''' Request authorization. If password is None and we are connecting
            to localhost, try to read password from the local config file
            GUI_RPC_PASSWD_FILE. If file can't be read (not found or no
            permission to read), try to authorize with a blank password.
            If authorization is requested and fails, all subsequent calls
            will be refused with socket.error 'Connection reset by peer' (104).
            Since most local calls do no require authorization, do not attempt
            it if you're not sure about the password.
        '''
        if password is None and not self.hostname:
            password = read_gui_rpc_password() or ""
        nonce = self.rpc.call('<auth1/>').text
        authhash = hashlib.md5('{0}{1}'.format(nonce, password).encode()).hexdigest().lower()
        reply = self.rpc.call('<auth2><nonce_hash>{0}</nonce_hash></auth2>'.format(authhash))

        if reply.tag == 'authorized':
            return True
        else:
            return False

    def exchange_versions(self):
        ''' Return VersionInfo instance with core client version info '''
        return VersionInfo.parse(self.rpc.call('<exchange_versions/>'))

    def get_tasks(self):
        ''' Same as get_results(active_only=False) '''
        return self.get_results(False)

    def get_results(self, active_only=False):
        ''' Get a list of results.
            Those that are in progress will have information such as CPU time
            and fraction done. Each result includes a name;
            Use CC_STATE::lookup_result() to find this result in the current static state;
            if it's not there, call get_state() again.
        '''
        reply = self.rpc.call("<get_results><active_only>{0}</active_only></get_results>".format(1 if active_only else 0))
        if not reply.tag == 'results':
            return []

        results = []
        for item in list(reply):
            results.append(Result.parse(item))

        return results


def read_gui_rpc_password():
    ''' Read password string from GUI_RPC_PASSWD_FILE file, trim the last CR
        (if any), and return it
    '''
    try:
        with open(GUI_RPC_PASSWD_FILE, 'r') as f:
            buf = f.read()
            if buf.endswith('\n'): return buf[:-1]  # trim last CR
            else: return buf
    except IOError:
        # Permission denied or File not found.
        pass
