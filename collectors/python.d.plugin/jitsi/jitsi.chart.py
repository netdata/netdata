# -*- coding: utf-8 -*-
# Description: jitsi videobridge netdata python.d module
# Author: François Deppierraz (ctrlaltdel)
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Installation
#
# Private REST API needs to be configured first:
#
#         1. Set 'JVB_OPTS="--apis=rest"' in /etc/jitsi/videobridge/config
#
#         2. Set 'org.jitsi.videobridge.rest.private.jetty.host=127.0.0.1' in
#            /etc/jitsi/videobridge/sip-communicator.properties
#
# Then simply copy this file into /usr/libexec/netdata/python.d/jitsi.chart.py
# and restart netdata.

from json import loads
from bases.FrameworkServices.UrlService import UrlService

priority = 99999

# CHART = {
#    id: {
#        'options': [name, title, units, family, context, charttype],
#        'lines': [
#            [unique_dimension_name, name, algorithm, multiplier, divisor]
#        ]}

ORDER = [
    "threads",
    "bitrate",
    "packetrate",
    "lossrate",
    "rtp_loss",
    "averages",
    "largest_conference",
    "channels",
    "total_loss",
    "total_conference_seconds",
    "total_conferences",
    "data_messages",
    "colibri_messages",
]

# Metric list comes from https://github.com/jitsi/jitsi-videobridge/blob/master/doc/statistics.md

CHARTS = {
    # threads - The number of Java threads that the video bridge is using.
    "threads": {
        "options": [
            None,
            "The number of Java threads that the video bridge is using.",
            "threads",
            "threads",
            "threads",
            "line",
        ],
        "lines": [["threads"]],
    },
    # bit_rate_download / bit_rate_upload - the total incoming and outgoing (respectively) bitrate for the video bridge in kilobits per second.
    "bitrate": {
        "options": [
            None,
            "The total bitrate for the video bridge.",
            "kilobits/s",
            "bitrate",
            "bitrate",
            "line",
        ],
        "lines": [["bit_rate_download", "download"], ["bit_rate_upload", "upload"]],
    },
    # packet_rate_download / packet_rate_upload - the total incoming and outgoing (respectively) packet rate for the video bridge in packets per second.
    "packetrate": {
        "options": [
            None,
            "The total packet rate for the video bridge.",
            "packets/s",
            "packetrate",
            "packetrate",
            "line",
        ],
        "lines": [
            ["packet_rate_download", "download"],
            ["packet_rate_upload", "upload"],
        ],
    },
    # loss_rate_download - The fraction of lost incoming RTP packets. This is based on RTP sequence numbers and is relatively accurate.
    # loss_rate_upload - The fraction of lost outgoing RTP packets. This is based on incoming RTCP Receiver Reports, and an attempt to subtract the fraction of packets that were not sent (i.e. were lost before they reached the bridge). Further, this is averaged over all streams of all users as opposed to all packets, so it is not correctly weighted. This is not accurate, but may be a useful metric nonetheless.
    "lossrate": {
        "options": [
            None,
            "The fraction of lost RTP packets.",
            "packets/s",
            "packetrate",
            "packetrate",
            "line",
        ],
        "lines": [["loss_rate_download", "download"], ["loss_rate_upload", "upload"]],
    },
    # rtp_loss - Deprecated. The sum of loss_rate_download and loss_rate_upload.
    "rtp_loss": {
        "options": [
            None,
            "The sum of loss_rate_download and loss_rate_upload.",
            "packets/s",
            "rtp_loss",
            "rtp_loss",
            "line",
        ],
        "lines": [["rtp_loss"]],
    },
    # jitter_aggregate - Experimental. An average value (in milliseconds) of the jitter calculated for incoming and outgoing streams. This hasn't been tested and it is currently not known whether the values are correct or not.
    # rtt_aggregate - An average value (in milliseconds) of the RTT across all streams.
    "averages": {
        "options": [
            None,
            "Averages across all streams.",
            "milliseconds",
            "averages",
            "averages",
            "line",
        ],
        "lines": [["jitter_aggregate", "jitter"], ["rtt_aggregate", "rtt"]],
    },
    # largest_conference - The number of participants in the largest conference currently hosted on the bridge.
    "largest_conference": {
        "options": [
            None,
            "The number of participants in the largest conference currently hosted on the bridge.",
            "participants",
            "largest_conference",
            "largest_conference",
            "line",
        ],
        "lines": [["largest_conference"]],
    },
    # TODO: conference_sizes - The distribution of conference sizes hosted on the bridge. It is an array of integers of size 15, and the value at (zero-based) index i is the number of conferences with i participants. The last element (index 14) also includes conferences with more than 14 participants.
    # audiochannels - The current number of audio channels.
    # videochannels - The current number of video channels.
    # conferences - The current number of conferences.
    # participants - The current number of participants.
    # videostreams - An estimation of the number of current video streams forwarded by the bridge.
    "channels": {
        "options": [
            None,
            "The number of channels.",
            "channels",
            "channels",
            "channels",
            "line",
        ],
        "lines": [
            ["audiochannels", "audio"],
            ["videochannels", "video"],
            ["conferences"],
            ["participants"],
            ["videostreams"],
        ],
    },
    # total_loss_controlled_participant_seconds -- The total number of participant-seconds that are loss-controlled.
    # total_loss_limited_participant_seconds -- The total number of participant-seconds that are loss-limited.
    # total_loss_degraded_participant_seconds -- The total number of participant-seconds that are loss-degraded.
    "total_loss": {
        "options": [
            None,
            "The total number of participant-seconds with loss.",
            "seconds",
            "total_loss",
            "total_loss",
            "line",
        ],
        "lines": [
            ["total_loss_controlled_participant_seconds", "controlled"],
            ["total_loss_limited_participant_seconds", "limited"],
            ["total_loss_degraded_participant_seconds", "degraded"],
        ],
    },
    # total_conference_seconds - The sum of the lengths of all completed conferences, in seconds.
    "total_conference_seconds": {
        "options": [
            None,
            "The sum of the lengths of all completed conferences.",
            "seconds",
            "total_conference_seconds",
            "total_conference_seconds",
            "line",
        ],
        "lines": [["total_conference_seconds"]],
    },
    # total_conferences_created - The total number of conferences created on the bridge.
    # total_failed_conferences - The total number of failed conferences on the bridge. A conference is marked as failed when all of its channels have failed. A channel is marked as failed if it had no payload activity.
    # total_partially_failed_conferences - The total number of partially failed conferences on the bridge. A conference is marked as partially failed when some of its channels has failed. A channel is marked as failed if it had no payload activity.
    "total_conferences": {
        "options": [
            None,
            "The total number of conferences on the bridge.",
            "conferences",
            "total_conferences",
            "total_conferences",
            "line",
        ],
        "lines": [
            ["total_conferences_created", "created"],
            ["total_failed_conferences", "failed"],
            ["total_partially_failed_conferences", "partially_failed"],
        ],
    },
    # total_data_channel_messages_received / total_data_channel_messages_sent - The total number messages received and sent through data channels.
    "data_messages": {
        "options": [
            None,
            "The total number messages received and sent through data channels.",
            "data_messages",
            "data_messages",
            "data_messages",
            "line",
        ],
        "lines": [
            ["total_data_channel_messages_received", "received"],
            ["total_data_channel_messages_sent", "sent"],
        ],
    },
    # total_colibri_web_socket_messages_received / total_colibri_web_socket_messages_sent - The total number messages received and sent through COLIBRI web sockets.
    "colibri_messages": {
        "options": [
            None,
            "The total number messages received and sent through COLIBRI web sockets.",
            "colibri_messages",
            "colibri_messages",
            "colibri_messages",
            "line",
        ],
        "lines": [
            ["total_colibri_web_socket_messages_received", "received"],
            ["total_colibri_web_socket_messages_sent", "sent"],
        ],
    },
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = self.configuration.get("url", "http://127.0.0.1:8080")

    def _get_data(self):
        raw_data = self._get_raw_data(self.url + "/colibri/stats")

        if raw_data is None:
            return None

        data = loads(raw_data)

        return data
