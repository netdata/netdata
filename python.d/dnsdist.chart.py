# -*- coding: utf-8 -*-
from json import loads
from bases.FrameworkServices.UrlService import UrlService

ORDER = ['latency', 'cache', 'acl', 'noncompliant', 'queries', 'health']
CHARTS = {
	'latency': {
		'options': [None, 'response latency', 'value', 'latency', 'dnsdist.latency', 'area'],
		'lines': [
			['latency-slow', '> 1sec', 'incremental'],
			['latency100-1000', '100-1000ms', 'incremental'],
			['latency50-100', '50-100ms', 'incremental'],
			['latency10-50', '10-50ms', 'incremental'],
			['latency1-10', '1-10ms', 'incremental'],
			['latency0-1', '< 1ms', 'incremental']
	]},
	'cache': {
		'options': [None, 'cache performance', 'value', 'cache', 'dnsdist.cache', 'area'],
		'lines': [
			['cache-hits', 'hits', 'incremental'],
			['cache-misses', 'misses', 'incremental']
	]},
	'acl': {
		'options': [None, 'access-control-list events', 'value', 'acl', 'dnsdist.acl', 'area'],
		'lines': [
			['acl-drops', 'drop by acl', 'incremental'],
			['rule-drop', 'drop by rule', 'incremental'],
			['rule-nxdomain', 'nxdomain by rule', 'incremental'],
			['rule-refused', 'refused by rule', 'incremental']
	]},
	'noncompliant': {
		'options': [None, 'noncompliant data', 'value', 'noncompliant', 'dnsdist.noncompliant', 'area'],
		'lines': [
			['empty-queries', 'empty queries', 'incremental'],
		    ['no-policy', 'no policy', 'incremental'],
			['noncompliant-queries', 'noncompliant queries', 'incremental'],
			['noncompliant-responses', 'noncompliant responses', 'incremental']
	]},
	'queries': {
		'options': [None, 'queries', 'value', 'queries', 'dnsdist.queries', 'area'],
		'lines': [
			['queries', 'queries', 'incremental'],
			['rdqueries', 'recursive queries', 'incremental'],
			['responses', 'responses', 'incremental']
	]},
	'health': {
		'options': [None, 'health', 'value', 'health', 'dnsdist.health', 'area'],
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

