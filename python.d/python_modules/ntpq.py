#!/usr/bin/env python3

# pragma pylint: disable=bad-whitespace
###############################################################################
# ntpq.py - Python NTP control library.
# Copyright (C) 2017 Sven Maeder (@rda0 on github)
# Copyright (C) 2016 Peter C. Norton (@pcn on github)
#
# this addition to ntplib is free software; you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License as published by the
# Free Software Foundation; either version 2 of the License, or (at your option)
# any later version.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS
# FOR A PARTICULAR PURPOSE.  See the GNU Lesser General Public License for more
# details.
#
# You should have received a copy of the GNU Lesser General Public License along
# with this program; if not, write to the Free Software Foundation, Inc., 59
# Temple Place, Suite 330, Boston, MA 0.1.2-1307 USA
###############################################################################
"""Python NTP control client implementation

This emulates the action fo the ntpq library, specifically two of the
read commands, "associations" and "peers".

"""

import socket
import struct
import time
import sys

import ntplib

NTP_CONTROL_PACKET_FORMAT = "!B B H H H H H" #Maybe, maybe not
"""packet format to pack/unpack"""

NTP_CONTROL_OPCODES = {
    "readstat" : 1,
    "readvar"  : 2
}


# From the ntp distributions ntpq/ntp_control.h
#define	CTL_PST_SEL_REJECT	0	/*   reject */
#define	CTL_PST_SEL_SANE	1	/* x falsetick */
#define	CTL_PST_SEL_CORRECT	2	/* . excess */
#define	CTL_PST_SEL_SELCAND	3	/* - outlier */
#define	CTL_PST_SEL_SYNCCAND	4	/* + candidate */
#define	CTL_PST_SEL_EXCESS	5	/* # backup */
#define	CTL_PST_SEL_SYSPEER	6	/* * sys.peer */
#define	CTL_PST_SEL_PPS		7	/* o pps.peer */
NTP_PEER_SELECTION = {
    # This is the "peer_selection" in data returned from a readvar request
    #
    "reject"   : 0,
    "sane"     : 1,
    "correct"  : 2,
    "selcand"  : 3,
    "synccand" : 4,
    "excess"   : 5,
    "sys.peer" : 6,
    "pps.peer" : 7
}


# packet format to pack/unpack the control header
CONTROL_PACKET_FORMAT = "!B B H H H H H"


class NTPException(Exception):
    """Exception raised by this module."""
    pass


def decode_association(data):
    """
    Provided a 2 uchar of data, unpack the first uchar of associationID,
    and the second uchar of association data from that uchar

    test with  e.g. data set to:
    In [161]: struct.pack("!B B", 0b00010100,0b00011010)
    Out[161]: '\x14\x1a'

    This is the data for a single association.
    """
    unpacked = struct.unpack("!H B B", data)

    return {
        'association_id'     : unpacked[0],
        'peer_config'        : unpacked[1] >> 7 & 0x1,
        'peer_authenable'    : unpacked[1] >> 6 & 0x1,
        'peer_authentic'     : unpacked[1] >> 5 & 0x1,
        'peer_reach'         : unpacked[1] >> 4 & 0x1,
        'reserved'           : unpacked[1] >> 3 & 0x1,
        'peer_selection'     : unpacked[1] & 0x7,
        'peer_event_counter' : unpacked[2] >> 4 & 0xf,
        'peer_event_code'    : unpacked[2] & 0xf
    }


def control_data_payload(version=2, op='readstat', association_id=0, sequence=1):
    """Convert the requested arguments into a buffer that can be sent over a socket.
    to an ntp server.

    Returns:
    buffer representing this packet

    Raises:
    NTPException -- in case of invalid field
    """
    leap           = 0       # leap second indicator
    version        = version # protocol version
    mode           = 6       # mode 6 is the control mode
    response_bit   = 0       # request
    error_bit      = 0
    more_bit       = 0
    opcode         = NTP_CONTROL_OPCODES[op]
    sequence       = sequence
    status         = 0
    association_id = association_id
    offset         = 0
    count          = 0
    try:
        packed = struct.pack(
            NTP_CONTROL_PACKET_FORMAT,
            (leap << 6 | version << 3 | mode),
            (response_bit << 7 | error_bit << 6 | more_bit << 5 | opcode),
            sequence,
            status,
            association_id,
            offset,
            count)
        return packed
    except struct.error:
        raise NTPException("Invalid NTP packet fields.")


def get_socket(host, port='ntp'):
    # lookup server address
    addrinfo = socket.getaddrinfo(host, port)[0]
    family, sockaddr = addrinfo[0], addrinfo[4]

    # create the socket
    sock = socket.socket(family, socket.SOCK_DGRAM)

    return sock, sockaddr


def ntp_control_request(sock, sockaddr, version=2,  # pylint: disable=too-many-arguments,invalid-name
                        op="readvar", association_id=0, timeout=5):
    """Query a NTP server.

    Parameters:
    host    -- server name/address
    version -- NTP version to use
    port    -- server port
    timeout -- timeout on socket operations

    Returns:
    dictionary with ntp control info.  Specific data will vary based on the request type.
    """

    try:
        sock.settimeout(timeout)

        # create a control request packet
        sock.sendto(
            control_data_payload(
                op=op, version=version,
                association_id=association_id),
            sockaddr)

        # wait for the response - check the source address
        src_addr = None,
        while src_addr[0] != sockaddr[0]:
            response_packet, src_addr = sock.recvfrom(512)

        # build the destination timestamp
        # dest_timestamp = ntplib.system_to_ntp_time(time.time())
    except socket.timeout:
        raise NTPException("No response received from %s." % host)
        sock.close()

    return response_packet


def control_packet_from_data(data):
    """Populate this instance from a NTP packet payload received from
    the network.

    Parameters:
    data -- buffer payload

    Returns:
    dictionary of control packet data.

    Raises:
    NTPException -- in case of invalid packet format
    """

    def decode_readstat(header_len, data, rdata):
        """
        Decodes a readstat request.  Augments rdata with
        association IDs from data
        """
        rdata['associations'] = list()
        for offset in range(header_len, len(data), 4):
            assoc = data[offset:offset+4]
            association_dict = decode_association(assoc)
            rdata['associations'].append(association_dict)
        return rdata

    def decode_readvar(header_len, data, rdata):
        """
        Decodes a redvar request.  Augments rdata dictionary with
        the textual data int he data packet.
        """
        def to_time(integ, frac, n=32): # pylint: disable=invalid-name
            """Return a timestamp from an integral and fractional part.
            Having this here eliminates using an function internal to
            ntplib.

            Parameters:
            integ -- integral part
            frac  -- fractional part
            n     -- number of bits of the fractional part
            Retuns:
            float seconds since the epoch/ aka a timestmap
            """
            return integ + float(frac)/2**n

        data = str(data).replace("\\r\\n"," ")
        buf = data[header_len:].split(",")
        for field in buf:
            try:
                key, val = field.replace("\r\n", "").lstrip().split("=")
            except ValueError as ve:
                sys.stderr.write("Got {} trying to unpack {}\n".format(ve, field))
                sys.stderr.write("as part of {}\n".format(buf))
                continue

            if key in ('rec', 'reftime'):
                int_part, frac_part = [ int(x, 16) for x in val.split(".") ]
                rdata[key] = ntplib.ntp_to_system_time(
                    to_time(int_part, frac_part))  # pylint: disable=protected-access
            else:
                rdata[key] = val
        # For the equivalent of the 'when' column, in ntpq -c pe
        # I believe that the time.time() minus the 'rec' matches that value.
        # Causes exception if associd = 0
        #rdata['when'] = time.time() - rdata['rec']
        return rdata

    try:
        header_len = struct.calcsize(CONTROL_PACKET_FORMAT)
        unpacked = struct.unpack(CONTROL_PACKET_FORMAT, data[0:header_len])
    except struct.error:
        raise NTPException("Invalid NTP packet.")

    # header status
    rdata = {
        "leap_header"          : unpacked[0] >> 6 & 0x1,
        "version"              : unpacked[0] >> 3 & 0x7,
        "mode"                 : unpacked[0] & 0x7,  # end first uchar
        "response_bit"         : unpacked[1] >> 7 & 0x1,
        "error_bit"            : unpacked[1] >> 6 & 0x1,
        "more_bit"             : unpacked[1] >> 5 & 0x1,
        "opcode"               : unpacked[1] & 0x1f,  # end second uchar
        "sequence"             : unpacked[2],
        "leap"                 : unpacked[3] >> 14 & 0x1,
        "clocksource"          : unpacked[3] >> 8 & 0x1f,  # 6 bit mask
        "system_event_counter" : unpacked[3] >> 4 & 0xf,
        "system_event_code"    : unpacked[3] & 0xf,  # End first ushort
        "association_id"       : unpacked[4],
        "offset"               : unpacked[5],
        "count"                : unpacked[6]
    }

    opcodes_by_number = { v:k for k, v in list(NTP_CONTROL_OPCODES.items()) }
    if opcodes_by_number[rdata['opcode']] == "readstat":
        return decode_readstat(header_len,  data, rdata)
    elif opcodes_by_number[rdata['opcode']] == "readvar":
        return decode_readvar(header_len,  data, rdata)
