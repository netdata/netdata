# -*- coding: utf-8 -*-
# Description: vnstat netdata python.d module
# Author: Noah Troy	(NoahTroy)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.ExecutableService import ExecutableService

VNSTAT_BASE_COMMAND = 'vnstat -i '

ORDER = [
			'total',
			'rxVtx',
			'avgRate',
		]

CHARTS =	{
				'total' :	{
								'options' : [None , 'Total Bandwidth Used' , 'GiB' , 'total bandwidth used' , 'vnstat.total' , 'line'],
								'lines' :	[
												['data']
											]
							},
				'rxVtx' :	{
								'options' :	[None , 'Total RX vs Total TX' , 'GiB' , 'total rx vs total tx' , 'vnstat.txVtx' , 'line'],
								'lines' :	[
												['rx data'],
												['tx data']
											]
							},
				'avgRate' :	{
								'options' :	[None , 'Average Daily vs Monthly Data Rate' , 'Mbit/s' , 'average daily vs monthly data rate' , 'line'],
								'lines' :	[
												['today\'s data rate'],
												['last month\'s data rate']
											]
							}
			}


class Service(ExecutableService):
	def __init__(self , configuration = None , name = None):
		ExecutableService.__init__(self , configuration = configuration , name = name)
        self.order = ORDER
        self.definitions = CHARTS
        self.command = (VNSTAT_BASE_COMMAND + self.configuration.get('interface' , 'eth0'))

    def _get_data(self):
        """
        Parse vnstat command output
        :return: dict
        """

		totalUnit = 'GiB'
		rxUnit = 'GiB'
		txUnit = 'GiB'
		dayUnit = 'Mbit/s'
		monthUnit = 'Mbit/s'
		totalData = 0.0
		totalRX = 0.0
		totalTX = 0.0
		dayAvgRate = 0.0
		monthAvgRate = 0.0

        try:
            rawOutput = self._get_raw_data()

			for line in rawOutput:
				if ('total:' in line):
					dataPoints = line.split()
					totalRX = float(dataPoints[1])
					rxUnit = dataPoints[2]
					totalTX = float(dataPoints[4])
					txUnit = dataPoints[5]
					totalData = float(dataPoints[7])
					totalUnit = dataPoints[6]
				if ('/s' in line):
					dataPoints = line.split()
					monthUnit = dataPoints[(len(dataPoints) - 1)]
					monthAvgRate = float(dataPoints[(len(dataPoints) - 2)])
				if ('today' in line):
					dataPoints = line.split()
					dayUnit = dataPoints[(len(dataPoints) - 1)]
					dayAvgRate = float(dataPoints[(len(dataPoints) - 2)])
					break

			self.definitions['total']['options'] = [None , 'Total Bandwidth Used' , totalUnit , 'total bandwidth used' , 'vnstat.total' , 'line']
			self.definitions['rxVtx']['options'] = [None , 'Total RX vs Total TX' , rxUnit , 'total rx vs total tx' , 'vnstat.txVtx' , 'line']
			self.definitions['rxVtx']['lines'] = [[('rx data (' + rxUnit + ')')] , [('tx data (' + txUnit + ')')]]
			self.definitions['avgRate']['options'] = [None , 'Average Daily vs Monthly Data Rate' , monthUnit , 'average daily vs monthly data rate' , 'line']
			self.definitions['avgRate']['lines'] = [[('today (' + dayUnit + ')')] , [('last month (' + monthUnit + ')')]]

            return {'data' : totalData , self.definitions['rxVtx']['lines'][0][0] : totalRX , self.definitions['rxVtx']['lines'][1][0] : totalTX , self.definitions['avgRate']['lines'][0][0] : dayAvgRate , self.definitions['avgRate']['lines'][1][0] : monthAvgRate}

        except (ValueError , AttributeError):
            return None
