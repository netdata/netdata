# -*- coding: utf-8 -*-
# Description:
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0-or-later

import os

from subprocess import Popen, PIPE

from bases.FrameworkServices.SimpleService import SimpleService
from bases.collection import find_binary


class ExecutableService(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.command = None

    def _get_raw_data(self, stderr=False, command=None):
        """
        Get raw data from executed command
        :return: <list>
        """
        try:
            p = Popen(command if command else self.command, stdout=PIPE, stderr=PIPE)
        except Exception as error:
            self.error('Executing command {command} resulted in error: {error}'.format(command=command or self.command,
                                                                                       error=error))
            return None
        data = list()
        std = p.stderr if stderr else p.stdout
        for line in std:
            try:
                data.append(line.decode('utf-8'))
            except TypeError:
                continue

        return data or None

    def check(self):
        """
        Parse basic configuration, check if command is whitelisted and is returning values
        :return: <boolean>
        """
        # Preference: 1. "command" from configuration file 2. "command" from plugin (if specified)
        if 'command' in self.configuration:
            self.command = self.configuration['command']

        # "command" must be: 1.not None 2. type <str>
        if not (self.command and isinstance(self.command, str)):
            self.error('Command is not defined or command type is not <str>')
            return False

        # Split "command" into: 1. command <str> 2. options <list>
        command, opts = self.command.split()[0], self.command.split()[1:]

        # Check for "bad" symbols in options. No pipes, redirects etc.
        opts_list = ['&', '|', ';', '>', '<']
        bad_opts = set(''.join(opts)) & set(opts_list)
        if bad_opts:
            self.error("Bad command argument(s): {opts}".format(opts=bad_opts))
            return False

        # Find absolute path ('echo' => '/bin/echo')
        if '/' not in command:
            command = find_binary(command)
            if not command:
                self.error('Can\'t locate "{command}" binary'.format(command=self.command))
                return False
        # Check if binary exist and executable
        else:
            if not os.access(command, os.X_OK):
                self.error('"{binary}" is not executable'.format(binary=command))
                return False

        self.command = [command] + opts if opts else [command]

        try:
            data = self._get_data()
        except Exception as error:
            self.error('_get_data() failed. Command: {command}. Error: {error}'.format(command=self.command,
                                                                                       error=error))
            return False

        if isinstance(data, dict) and data:
            return True
        self.error('Command "{command}" returned no data'.format(command=self.command))
        return False
