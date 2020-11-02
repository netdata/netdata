# -*- coding: utf-8 -*-
# Description: zoneminder netdata python.d module
# Author: Jose Chapa
# SPDX-License-Identifier: GPL-3.0-or-later
# Zoneminder API: https://zoneminder.readthedocs.io/en/stable/api.html

import time
import os.path

try:
    import requests
    import jwt

    HAVE_DEPS = True
except ImportError:
    HAVE_DEPS = False

from bases.FrameworkServices.SimpleService import SimpleService

update_every = 10

ORDER = [
    'camera_fps',
    'camera_bandwidth',
    'events',
    'disk_usage',
]

CHARTS = {
    'camera_fps': {
        'options': [None, 'Capture FPS', 'FPS', 'capture_fps', 'camera.fps', 'line'],
        'lines': []
    },
    'camera_bandwidth': {
        'options': [None, 'Capture Bandwidth', 'kB/s', 'camera_bandwidth', 'camera.bandwidth', 'stacked'],
        'lines': []
    },
    'events': {
        'options': [None, 'Events', 'count', 'events', 'camera.events', 'stacked'],
        'lines': []
    },
    'disk_usage': {
        'options': [None, 'Disk Space', 'GB', 'disk_space', 'camera.disk_space', 'area'],
        'lines': [
            ['disk_space', 'used', 'absolute', None, 1073741824]
        ]
    },
}


def generate_refresh_token(url, user, password, connection_timeout):
    try:
        post_data = dict()
        post_data["user"] = user
        post_data["pass"] = password
        r = requests.post(url + '/api/host/login.json', data=post_data, timeout=connection_timeout,
                          verify=False)
        json_data = r.json()
        if all(k in json_data for k in ("access_token", "refresh_token")):
            try:
                token_file = open(os.path.expanduser("~/.zm_token.txt"), 'w')
                token_file.write("{}|{}".format(json_data["access_token"], json_data["refresh_token"]))
                token_file.close()
            except IOError:
                return "<error>", "Error while writing ~/.zm_token.txt file."
            return "ok", "{}|{}".format(json_data["access_token"], json_data["refresh_token"])
        return "<error>", "Invalid api response when trying to generate new access and refresh tokens: " + r.text
    except requests.exceptions.RequestException as e:
        return "<error>", e


def generate_access_token(url, refresh_token, connection_timeout):
    try:
        r = requests.post(url + '/api/host/login.json?token=' + refresh_token, timeout=connection_timeout,
                          verify=False)
        json_data = r.json()
        if "access_token" in json_data:
            try:
                token_file = open(os.path.expanduser("~/.zm_token.txt"), 'w')
                token_file.write("{}|{}".format(json_data["access_token"], refresh_token))
                token_file.close()
            except IOError:
                return "<error>", "Error while writing ~/.zm_token.txt file."
            return "ok", json_data["access_token"]
        return "<error>", "Invalid api response when trying to generate new access token: " + r.text
    except requests.exceptions.RequestException as e:
        return "<error>", e


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = self.configuration.get("url", "http://127.0.0.1/zm")
        self.url = self.url.strip('/')
        self.user = self.configuration.get("user", "")
        self.password = self.configuration.get("pass", "")
        self.connection_timeout = self.configuration.get("timeout", 10)

    def check(self):
        if not HAVE_DEPS:
            self.error("'requests' and 'PyJWT' python packages are needed.")
            return False
        return True

    def _get_data(self):
        data = dict()
        access_token = refresh_token = ''
        bool_login = True
        disk_space = 0

        # if user is not defined, then do not attempt to login
        if not self.user:
            bool_login = False

        # get access token from file or zoneminder api
        if bool_login:
            try:
                token_file = open(os.path.expanduser("~/.zm_token.txt"))
                access_token, refresh_token = token_file.read().split('|')
                token_file.close()
            except IOError:
                result, output = generate_refresh_token(self.url, self.user, self.password,
                                                           self.connection_timeout)
                if "<error>" in result:
                    self.debug("error: " + output)
                    return None
                self.debug("new access and refresh tokens were generated...")
                access_token, refresh_token = output.split('|')

            # get jwt information
            jwt_access_data = jwt.decode(access_token, verify=False)
            jwt_refresh_data = jwt.decode(refresh_token, verify=False)

            # get new refresh token if it expires in less than 30 minutes
            if (jwt_refresh_data['exp'] - time.time()) < 1800:
                self.debug("generating new refresh token...")
                result, output = generate_refresh_token(self.url, self.user, self.password,
                                                           self.connection_timeout)
                if "<error>" in result:
                    self.debug("error: " + output)
                    return None
                access_token, refresh_token = output.split('|')

            # get new access token if current token expires in less than 5 minutes
            if (jwt_access_data['exp'] - time.time()) < 300:
                result, output = generate_access_token(self.url, refresh_token, self.connection_timeout)
                if "<error>" in result:
                    self.debug("error: " + output)
                    return None
                access_token = output

        # get data from monitors api call
        try:
            r = requests.get(self.url + '/api/monitors.json?token=' + access_token,
                             timeout=self.connection_timeout, verify=False)
            json_data = r.json()
        except requests.exceptions.RequestException as e:
            self.debug(e)
            return None

        if all(k in json_data for k in ("success", "data")):
            if json_data['success'] == False and 'Token revoked' in json_data['data']['name']:
                self.debug("token was revoked, generating new tokens, will try to collect data in next run...")
                result, output = generate_refresh_token(self.url, self.user, self.password,
                                                           self.connection_timeout)
                if "<error>" in result:
                    self.debug("error: " + output)
                return None

        if "monitors" in json_data:
            for monitor in json_data["monitors"]:
                if "Monitor" in monitor:
                    try:
                        disk_space += float(monitor["Monitor"]["TotalEventDiskSpace"])
                    except Exception as e:
                        self.debug(e)
                    if monitor["Monitor"]["Function"] == "None" or monitor["Monitor"]["Enabled"] == "0":
                        continue
                    if "fps_" + monitor["Monitor"]["Id"] not in self.charts['camera_fps']:
                        self.charts['camera_fps'].add_dimension(
                            ["fps_" + monitor["Monitor"]["Id"], monitor["Monitor"]["Name"], 'absolute'])
                    if "bandwidth_" + monitor["Monitor"]["Id"] not in self.charts['camera_bandwidth']:
                        self.charts['camera_bandwidth'].add_dimension(
                            ["bandwidth_" + monitor["Monitor"]["Id"], monitor["Monitor"]["Name"], 'absolute', None,
                             1024])
                    if "events_" + monitor["Monitor"]["Id"] not in self.charts['events']:
                        self.charts['events'].add_dimension(
                            ["events_" + monitor["Monitor"]["Id"], monitor["Monitor"]["Name"], 'absolute'])
                    try:
                        data["fps_" + monitor["Monitor"]["Id"]] = float(monitor["Monitor_Status"]["CaptureFPS"])
                        data["bandwidth_" + monitor["Monitor"]["Id"]] = float(
                            monitor["Monitor_Status"]["CaptureBandwidth"])
                        data["events_" + monitor["Monitor"]["Id"]] = float(monitor["Monitor"]["TotalEvents"])
                    except Exception as e:
                        self.debug(e)
        else:
            self.debug("Invalid zoneminder api response: " + r.text)
            return None

        data["disk_space"] = disk_space

        return data
