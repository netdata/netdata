# -*- coding: utf-8 -*-
# Description: chronyd netdata python.d module
# Author: fulltux 

from base import SimpleService
from subprocess import Popen, PIPE
import pdb
# default module values (can be overridden per job in `config`)

update_every = 2
priority = 60000
retries = 60

ORDER= ['offset', 'frequency']
CHARTS={
        'offset': {
                'options': [None, "Chrony", "offset", 'size in ms', 'chrony.offset', 'area'],
                'lines': [
                ]},
        'frequency': {
                'options': [None, "Chrony frequency", "frequency", 'queue', 'chrony.frequency', 'line'],
                'lines': [
       	        ]}
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.command = "chronyc -n sourcestats"
	self.chart_name="chrony"
        self.order = ORDER
        self.definitions = CHARTS
        self._data_from_check = dict()
        self.create()

    def _get_raw_data(self):
        """
        Get raw data from executed command
        :return: <list>
        """
        try:
            p = Popen(["chronyc","-n","sourcestats"], stdout=PIPE, stderr=PIPE)
        except Exception as error:
            self.error("Executing command", self.command, "resulted in error:", str(error))
            return None
        data = list()
        for line in p.stdout.readlines():
            data.append(line.decode())
        return data or None

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        try:
            raws=self._get_raw_data()[3:]
            result=dict()
            for raw in raws:
                srv,NP ,NR ,Span ,Frequency ,Skew ,Offset, Dev= raw.split()

		if Offset[-2:] == "ms":
		    Offset=float(Offset[:-2])*1000
		elif Offset[-2:] == "ns":
		    Offset=float(Offset[:-2])/1000
		else :
		    Offset=float(Offset[:-2])
                result['o' + srv]= Offset
                result['f' + srv]= float(Frequency)
            return result

        except (ValueError, AttributeError):
            return None

    def create_charts(self):
        self.order = ORDER
        self.definitions = CHARTS
        for raw in self._get_raw_data()[3:]:
            srv= raw.split()[0]
            self.definitions['frequency']['lines'].append(["f"+srv, srv, 'absolute'])
            self.definitions['offset']['lines'].append(["o"+srv, srv, 'absolute'])


    def check(self):
        self.create_charts()
        return True

