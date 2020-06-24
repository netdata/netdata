# -*- coding: utf-8 -*-
# Description: vnstat netdata python.d module
# Author: Noah Troy    (NoahTroy)
# SPDX-License-Identifier: GPL-3.0-or-later
import json
from calendar import monthrange

from bases.FrameworkServices.ExecutableService import ExecutableService

VNSTAT_BASE_COMMAND = 'vnstat --json'

ORDER = [
            'nowVday_def',
            'hoursAvg_def',
            'hoursTot_def',
            'daysAvg_def',
            'daysTot_def',
            'monthsAvg_def',
            'monthsTot_def',
            'yearsAvg_def',
            'yearsTot_def',
            'totals_def'
        ]

CHARTS = {
                'nowVday_def' : {
                              'options' : [None , 'Current vs. Yesterday' , 'kilobits/s' , 'average data rate' , 'vnstat.nowVday_def' , 'stacked'],
                              'lines' : [
                                            ['nowVday_def_current' , 'now' , 'absolute' , 1 , 1],
                                            ['nowVday_def_yesterday' , 'yesterday' , 'absolute' , 1 , 1]
                                        ]
                          },
                'hoursAvg_def' : {
                                'options' : [None , 'Average Data Transfer Rate Per Hour' , 'kilobits/s' , 'hourly data rate' , 'vnstat.hoursAvg_def' , 'stacked'],
                                'lines' : [
                                              ['hoursAvg_def_hour1' , 'hour1' , 'absolute' , 1 , 1]
                                          ]
                            },
                'hoursTot_def' : {
                                'options' : [None , 'Total Data Transfer Per Hour' , 'B' , 'hourly transfer' , 'vnstat.hoursTot_def' , 'stacked'],
                                'lines' : [
                                              ['hoursTot_def_hour1' , 'hour1' , 'absolute' , 1 , 1]
                                          ]
                            },
                'daysAvg_def' : {
                                'options' : [None , 'Average Data Transfer Rate Per Day' , 'kilobits/s' , 'daily data rate' , 'vnstat.daysAvg_def' , 'stacked'],
                                'lines' : [
                                              ['daysAvg_def_day1' , 'day1' , 'absolute' , 1 , 1]
                                          ]
                            },
                'daysTot_def' : {
                                'options' : [None , 'Total Data Transfer Per Day' , 'B' , 'daily transfer' , 'vnstat.daysTot_def' , 'stacked'],
                                'lines' : [
                                              ['daysTot_def_day1' , 'day1' , 'absolute' , 1 , 1]
                                          ]
                            },
                'monthsAvg_def' : {
                                'options' : [None , 'Average Data Transfer Rate Per Month' , 'kilobits/s' , 'monthly data rate' , 'vnstat.monthsAvg_def' , 'stacked'],
                                'lines' : [
                                              ['monthsAvg_def_month1' , 'month1' , 'absolute' , 1 , 1]
                                          ]
                            },
                'monthsTot_def' : {
                                'options' : [None , 'Total Data Transfer Per Month' , 'B' , 'monthly transfer' , 'vnstat.monthsTot_def' , 'stacked'],
                                'lines' : [
                                              ['monthsTot_def_month1' , 'month1' , 'absolute' , 1 , 1]
                                          ]
                            },
                'yearsAvg_def' : {
                                'options' : [None , 'Average Data Transfer Rate Per Year' , 'kilobits/s' , 'yearly data rate' , 'vnstat.yearsAvg_def' , 'stacked'],
                                'lines' : [
                                              ['yearsAvg_def_year1' , 'year1' , 'absolute' , 1 , 1]
                                          ]
                            },
                'yearsTot_def' : {
                                'options' : [None , 'Total Data Transfer Per Year' , 'B' , 'yearly transfer' , 'vnstat.yearsTot_def' , 'stacked'],
                                'lines' : [
                                              ['yearsTot_def_year1' , 'year1' , 'absolute' , 1 , 1]
                                          ]
                            },
                'totals_def' : {
                              'options' : [None , 'Total RX vs. Total TX' , 'B' , 'total transfer' , 'vnstat.totals_def' , 'stacked'],
                              'lines' : [
                                            ['totals_def_rx' , 'rx', 'absolute' , 1 , 1],
                                            ['totals_def_tx' , 'tx' , 'absolute' , 1 , 1]
                                        ]
                          }
         }


class Service(ExecutableService):
    def __init__(self , configuration = None , name = None):
        ExecutableService.__init__(self , configuration = configuration , name = name)
        self.order = ORDER
        self.definitions = CHARTS
        self.command = VNSTAT_BASE_COMMAND


    def genChartsAndValues(self , interfaces , dataPoints):
        charts = {}
        order = []
        data = dict()

        try:
            hoursLimit = int(self.configuration.get('hours_limit' , 0))
            daysLimit = int(self.configuration.get('days_limit' , 0))
            monthsLimit = int(self.configuration.get('months_limit' , 0))
            yearsLimit = int(self.configuration.get('years_limit' , 0))
        except Exception as msg:
            self.debug(str(msg))
            self.debug('You are most-likely seeing the above message because "hours_limit", "days_limit", "months_limit", or "years_limit" contained invalid (non-integer) entries. The default values will be used instead...')
            hoursLimit = 0
            daysLimit = 0
            monthsLimit = 0
            yearsLimit = 0

        try:
            dataRep = int(self.configuration.get('data_representation' , 1))
            if (not((dataRep == 0) or ((dataRep == 1) or (dataRep == 2)))):
                dataRep = 1
        except Exception as msg:
            self.debug(str(msg))
            self.debug('You are most-likely seeing the above message because "data_representation" contains an invalid (non-integer) entry. The default value will be used instead...')
            dataRep = 1

        try:
            chartsEnabled = int(self.configuration.get('enable_charts' , 0))
            if (not((chartsEnabled == 1) and (len(interfaces) == 1))):
                chartsEnabled = 0
        except Exception as msg:
            self.debug(str(msg))
            self.debug('You are most-likely seeing the above message because "enable_charts" contains an invalid (non-integer) entry. The default value will be used instead...')
            chartsEnabled = 0

        index = 0
        for interface in interfaces:

            intOrChart = interface
            if (chartsEnabled == 1):
                intOrChart = 'chart'


            charts[('nowVday_' + intOrChart)] = {'options' : [None , 'Current vs. Yesterday' , 'kilobits/s' , ('average data rate ' + interface) , ('vnstat.nowVday_' + intOrChart) , 'stacked'] , 'lines' : [[('nowVday_' + interface + '_current') , 'current' , 'absolute' , 1 , 1] , [('nowVday_' + interface + '_yesterday') , 'yesterday' , 'absolute' , 1 , 1]]}

            order.append(('nowVday_' + intOrChart))

            data[('nowVday_' + interface + '_current')] = dataPoints[index][2]
            data[('nowVday_' + interface + '_yesterday')] = dataPoints[index][3]


            if (hoursLimit != 0):
                if ((dataRep == 0) or (dataRep == 2)):
                    charts[('hoursAvg_' + intOrChart)] = {'options' : [None , 'Average Data Transfer Rate Per Hour' , 'kilobits/s' , ('hourly data rate ' + interface) , ('vnstat.hoursAvg_' + intOrChart) , 'stacked'] , 'lines' : []}

                    order.append(('hoursAvg_' + intOrChart))

                if ((dataRep == 0) or (dataRep == 1)):
                    charts[('hoursTot_' + intOrChart)] = {'options' : [None , 'Total Data Transfer Per Hour' , 'B' , ('hourly transfer ' + interface) , ('vnstat.hoursTot_' + intOrChart) , 'stacked'] , 'lines' : []}

                    order.append(('hoursTot_' + intOrChart))

                startAt = (len(dataPoints[index][4]) - hoursLimit)
                if ((hoursLimit == -1) or (startAt < 0)):
                    startAt = 0
                for hour in dataPoints[index][4][startAt:]:
                    if ((dataRep == 0) or (dataRep == 2)):
                        totalAmount = hour[1]
                        averageAmount = (((totalAmount * 8) / 1000) / 3600)
                        newLine = [('hoursAvg_' + interface + '_hour' + str(hash(hour[0]))[1:]) , hour[0] , 'absolute' , 1 , 1]
                        charts[('hoursAvg_' + intOrChart)]['lines'].append(newLine)

                        try:
                            self.charts[('hoursAvg_' + intOrChart)].add_dimension(newLine)
                        except Exception as msg:
                            # This value either already exists or the chart has not been instantiated yet
                            self.debug(str(msg))

                        data[('hoursAvg_' + interface + '_hour' + str(hash(hour[0]))[1:])] = averageAmount

                    if ((dataRep == 0) or (dataRep == 1)):
                        newLine = [('hoursTot_' + interface + '_hour' + str(hash(hour[0]))[1:]) , hour[0] , 'absolute' , 1 , 1]
                        charts[('hoursTot_' + intOrChart)]['lines'].append(newLine)

                        try:
                            self.charts[('hoursTot_' + intOrChart)].add_dimension(newLine)
                        except Exception as msg:
                            # This value either already exists or the chart has not been instantiated yet
                            self.debug(str(msg))

                        data[('hoursTot_' + interface + '_hour' + str(hash(hour[0]))[1:])] = hour[1]


            if (daysLimit != 0):
                if ((dataRep == 0) or (dataRep == 2)):
                    charts[('daysAvg_' + intOrChart)] = {'options' : [None , 'Average Data Transfer Rate Per Day' , 'kilobits/s' , ('daily data rate ' + interface) , ('vnstat.daysAvg_' + intOrChart) , 'stacked'] , 'lines' : []}

                    order.append(('daysAvg_' + intOrChart))

                if ((dataRep == 0) or (dataRep == 1)):
                    charts[('daysTot_' + intOrChart)] = {'options' : [None , 'Total Data Transfer Per Day' , 'B' , ('daily transfer ' + interface) , ('vnstat.daysTot_' + intOrChart) , 'stacked'] , 'lines' : []}

                    order.append(('daysTot_' + intOrChart))

                startAt = (len(dataPoints[index][5]) - daysLimit)
                if ((daysLimit == -1) or (startAt < 0)):
                    startAt = 0
                for day in dataPoints[index][5][startAt:]:
                    if ((dataRep == 0) or (dataRep == 2)):
                        totalAmount = day[1]
                        averageAmount = (((totalAmount * 8) / 1000) / 86400)
                        newLine = [('daysAvg_' + interface + '_day' + str(hash(day[0]))[1:]) , day[0] , 'absolute' , 1 , 1]
                        charts[('daysAvg_' + intOrChart)]['lines'].append(newLine)

                        try:
                            self.charts[('daysAvg_' + intOrChart)].add_dimension(newLine)
                        except Exception as msg:
                            # This value either already exists or the chart has not been instantiated yet
                            self.debug(str(msg))

                        data[('daysAvg_' + interface + '_day' + str(hash(day[0]))[1:])] = averageAmount

                    if ((dataRep == 0) or (dataRep == 1)):
                        newLine = [('daysTot_' + interface + '_day' + str(hash(day[0]))[1:]) , day[0] , 'absolute' , 1 , 1]
                        charts[('daysTot_' + intOrChart)]['lines'].append(newLine)

                        try:
                            self.charts[('daysTot_' + intOrChart)].add_dimension(newLine)
                        except Exception as msg:
                            # This value either already exists or the chart has not been instantiated yet
                            self.debug(str(msg))

                        data[('daysTot_' + interface + '_day' + str(hash(day[0]))[1:])] = day[1]


            if (monthsLimit != 0):
                if ((dataRep == 0) or (dataRep == 2)):
                    charts[('monthsAvg_' + intOrChart)] = {'options' : [None , 'Average Data Transfer Rate Per Month' , 'kilobits/s' , ('monthly data rate ' + interface) , ('vnstat.monthsAvg_' + intOrChart) , 'stacked'] , 'lines' : []}

                    order.append(('monthsAvg_' + intOrChart))

                if ((dataRep == 0) or (dataRep == 1)):
                    charts[('monthsTot_' + intOrChart)] = {'options' : [None , 'Total Data Transfer Per Month' , 'B' , ('monthly transfer ' + interface) , ('vnstat.monthsTot_' + intOrChart) , 'stacked'] , 'lines' : []}

                    order.append(('monthsTot_' + intOrChart))

                startAt = (len(dataPoints[index][6]) - monthsLimit)
                if ((monthsLimit == -1) or (startAt < 0)):
                    startAt = 0
                for month in dataPoints[index][6][startAt:]:
                    if ((dataRep == 0) or (dataRep == 2)):
                        totalAmount = month[1]
                        numDays = monthrange(int(month[0].split('/')[1]) , int(month[0].split('/')[0]))[1]
                        averageAmount = (((totalAmount * 8) / 1000) / (numDays * 86400))
                        newLine = [('monthsAvg_' + interface + '_month' + str(hash(month[0]))[1:]) , month[0] , 'absolute' , 1 , 1]
                        charts[('monthsAvg_' + intOrChart)]['lines'].append(newLine)

                        try:
                            self.charts[('monthsAvg_' + intOrChart)].add_dimension(newLine)
                        except Exception as msg:
                            # This value either already exists or the chart has not been instantiated yet
                            self.debug(str(msg))

                        data[('monthsAvg_' + interface + '_month' + str(hash(month[0]))[1:])] = averageAmount

                    if ((dataRep == 0) or (dataRep == 1)):
                        newLine = [('monthsTot_' + interface + '_month' + str(hash(month[0]))[1:]) , month[0] , 'absolute' , 1 , 1]
                        charts[('monthsTot_' + intOrChart)]['lines'].append(newLine)

                        try:
                            self.charts[('monthsTot_' + intOrChart)].add_dimension(newLine)
                        except Exception as msg:
                            # This value either already exists or the chart has not been instantiated yet
                            self.debug(str(msg))

                        data[('monthsTot_' + interface + '_month' + str(hash(month[0]))[1:])] = month[1]


            if (yearsLimit != 0):
                if ((dataRep == 0) or (dataRep == 2)):
                    charts[('yearsAvg_' + intOrChart)] = {'options' : [None , 'Average Data Transfer Rate Per Year' , 'kilobits/s' , ('yearly data rate ' + interface) , ('vnstat.yearsAvg_' + intOrChart) , 'stacked'] , 'lines' : []}

                    order.append(('yearsAvg_' + intOrChart))

                if ((dataRep == 0) or (dataRep == 1)):
                    charts[('yearsTot_' + intOrChart)] = {'options' : [None , 'Total Data Transfer Per Year' , 'B' , ('yearly transfer ' + interface) , ('vnstat.yearsTot_' + intOrChart) , 'stacked'] , 'lines' : []}

                    order.append(('yearsTot_' + intOrChart))

                startAt = (len(dataPoints[index][7]) - yearsLimit)
                if ((yearsLimit == -1) or (startAt < 0)):
                    startAt = 0
                for year in dataPoints[index][7][startAt:]:
                    if ((dataRep == 0) or (dataRep == 2)):
                        totalAmount = year[1]
                        numDays = 365
                        if (monthrange(int(year[0]) , 2)[1] == 29):
                            numDays = 366
                        averageAmount = (((totalAmount * 8) / 1000) / (numDays * 86400))
                        newLine = [('yearsAvg_' + interface + '_year' + str(hash(year[0]))[1:]) , year[0] , 'absolute' , 1 , 1]
                        charts[('yearsAvg_' + intOrChart)]['lines'].append(newLine)

                        try:
                            self.charts[('yearsAvg_' + intOrChart)].add_dimension(newLine)
                        except Exception as msg:
                            # This value either already exists or the chart has not been instantiated yet
                            self.debug(str(msg))

                        data[('yearsAvg_' + interface + '_year' + str(hash(year[0]))[1:])] = averageAmount

                    if ((dataRep == 0) or (dataRep == 1)):
                        newLine = [('yearsTot_' + interface + '_year' + str(hash(year[0]))[1:]) , year[0] , 'absolute' , 1 , 1]
                        charts[('yearsTot_' + intOrChart)]['lines'].append(newLine)

                        try:
                            self.charts[('yearsTot_' + intOrChart)].add_dimension(newLine)
                        except Exception as msg:
                            # This value either already exists or the chart has not been instantiated yet
                            self.debug(str(msg))

                        data[('yearsTot_' + interface + '_year' + str(hash(year[0]))[1:])] = year[1]


            charts[('totals_' + intOrChart)] = {'options' : [None , 'Total RX vs. Total TX' , 'B' , ('total bandwidth ' + interface) , ('vnstat.totals_' + intOrChart) , 'stacked'] , 'lines' : [[('totals_' + interface + '_rx') , 'rx' , 'absolute' , 1 , 1] , [('totals_' + interface + '_tx') , 'tx' , 'absolute' , 1 , 1]]}

            order.append(('totals_' + intOrChart))

            data[('totals_' + interface + '_rx')] = dataPoints[index][0]
            data[('totals_' + interface + '_tx')] = dataPoints[index][1]


            index += 1

        return charts , order , data


    def _get_data(self):
        """
        Parse vnstat command output
        :return: dict
        """

        try:
            rawOutput = str(self._get_raw_data()[0]).strip()
            output = json.loads(rawOutput)

            allowedInterfaces = self.configuration.get('interface' , 'all').split()

            interfaces = []
            dataPoints = []
            for interface in output['interfaces']:
                if ((not('all' in allowedInterfaces)) and (not(interface['name'] in allowedInterfaces))):
                    continue

                interfaces.append(interface['name'])

                traffic = interface['traffic']

                totalRX = traffic['total']['rx']
                totalTX = traffic['total']['tx']

                currentAmount = (traffic['fiveminute'][(len(traffic['fiveminute']) - 1)]['rx'] + traffic['fiveminute'][(len(traffic['fiveminute']) - 1)]['rx'])
                currentDataRate = (((currentAmount * 8) / 1000) / 300)
                # If no data is present for yesterday, then today's values will be chosen automatically:
                yesterdayAmount = (traffic['day'][(len(traffic['day']) - 2)]['rx'] + traffic['day'][(len(traffic['day']) - 2)]['tx'])
                yesterdayDataRate = (((yesterdayAmount * 8) / 1000) / 86400)

                hoursTotData = []
                for hour in traffic['hour']:
                    date = (str(hour['date']['day']) + '/' + str(hour['date']['month']) + '/' + str(hour['date']['year']))
                    minute = str(hour['time']['minute'])
                    if (len(minute) == 1):
                        minute = ('0' + minute)
                    time = (str(hour['time']['hour']) + ':' + minute)
                    hourTotal = (hour['rx'] + hour['tx'])
                    hoursTotData.append([(time + ' ' + date) , hourTotal])

                daysTotData = []
                for day in traffic['day']:
                    date = (str(day['date']['day']) + '/' + str(day['date']['month']) + '/' + str(day['date']['year']))
                    dayTotal = (day['rx'] + day['tx'])
                    daysTotData.append([date , dayTotal])

                monthsTotData = []
                for month in traffic['month']:
                    date = (str(month['date']['month']) + '/' + str(month['date']['year']))
                    monthTotal = (month['rx'] + month['tx'])
                    monthsTotData.append([date , monthTotal])

                yearsTotData = []
                for year in traffic['year']:
                    date = str(year['date']['year'])
                    yearTotal = (year['rx'] + year['tx'])
                    yearsTotData.append([date , yearTotal])

                thisIntData = [totalRX , totalTX , currentDataRate , yesterdayDataRate , hoursTotData , daysTotData , monthsTotData , yearsTotData]

                dataPoints.append(thisIntData)

            self.definitions , self.order , data = self.genChartsAndValues(interfaces , dataPoints)

            return data

        except Exception as msg:
            self.error(str(msg))
            return None
