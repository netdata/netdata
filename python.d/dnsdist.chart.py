# -*- coding: utf-8 -*-
from json import loads
from bases.FrameworkServices.UrlService import UrlService

ORDER = ['latency', 'cache', 'acl', 'noncompliant', 'queries', 'health']
CHARTS = {
	'latency': {
		'options': [None, 'latency stats', 'percent', 'latency', 'dnsdist.latency', 'area'],
		'lines': [
			['latency-slow', '> 1sec', 'incremental'],
			['latency100-1000', '100-1000ms', 'incremental'],
			['latency50-100', '50-100ms', 'incremental'],
			['latency10-50', '10-50ms', 'incremental'],
			['latency1-10', '1-10ms', 'incremental'],
			['latency0-1', '< 1ms', 'incremental']
	]},
	'cache': {
		'options': [None, 'cache stats', 'value', 'cache', 'dnsdist.cache', 'area'],
		'lines': [
			['cache-hits', 'hits', 'incremental'],
			['cache-misses', 'misses', 'incremental']
	]},
	'acl': {
		'options': [None, 'AccessControlList stats', 'value', 'acl', 'dnsdist.acl', 'area'],
		'lines': [
			['acl-drops', None, 'incremental'],
			['rule-drop', None, 'incremental'],
			['rule-nxdomain', None, 'incremental'],
			['rule-refused', None, 'incremental']
	]},
	'noncompliant': {
		'options': [None, 'Noncompliant data', 'value', 'noncompliant', 'dnsdist.noncompliant', 'area'],
		'lines': [
			['empty-queries', None, 'incremental'],
		    ['no-policy', None, 'incremental'],
			['noncompliant-queries', None, 'incremental'],
			['noncompliant-responses', None, 'incremental']
	]},
	'queries': {
		'options': [None, 'Relative query stats', 'value', 'queries', 'dnsdist.queries', 'area'],
		'lines': [
			['queries', None, 'incremental'],
			['rdqueries', None, 'incremental'],
			['responses', None, 'incremental']
	]},
	'health': {
		'options': [None, 'Health', 'value', 'health', 'dnsdist.health', 'area'],
		'lines': [
			['downstream-send-errors', 'ds send errors', 'incremental'],
			['downstream-timeouts', 'ds timeouts', 'incremental'],
			['servfail-responses', 'servfail responses', 'incremental'],
			['trunc-failures', 'trunc failures', 'incremental']
	]}
}

class Service(UrlService):
	def __init__(self, configuration=None, name=None):
		UrlService.__init__(self, configuration=configuration, name=name)
		self.order = ORDER
		self.definitions = CHARTS

	def _get_data(self):
		data = self._get_raw_data()
		if not data:
			return None
		
		return loads(data)
		
