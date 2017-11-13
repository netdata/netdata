# -*- coding: utf-8 -*-
from json import loads
from bases.FrameworkServices.UrlService import UrlService

ORDER = ['latency_rel', 'latency_abs', 'cache_rel', 'cache_abs', 'acl_abs', 'invalid_abs', 'queries_rel', 'queries_abs', 'health_abs']
CHARTS = {
	'latency_rel': {
		'options': [None, 'Average latency', 'value', 'Latency', 'dnsdist.latency_rel', 'area'],
		'lines': [
			['latency-slow', '> 1sec', 'incremental'],
			['latency100-1000', '100-1000ms', 'incremental'],
			['latency50-100', '50-100ms', 'incremental'],
			['latency10-50', '10-50ms', 'incremental'],
			['latency1-10', '1-10ms', 'incremental'],
			['latency0-1', '< 1ms', 'incremental']
	]},
	'latency_abs': {
		'options': [None, 'Absolute latency', 'value', 'Latency', 'dnsdist.latency_abs', 'line'],
		'lines': [
			['latency-slow', '> 1sec', 'absolute'],
			['latency100-1000', '100-1000ms', 'absolute'],
			['latency50-100', '50-100ms', 'absolute'],
			['latency10-50', '10-50ms', 'absolute'],
			['latency1-10', '1-10ms', 'absolute'],
			['latency0-1', '< 1ms', 'absolute']
	]},
	'cache_rel': {
		'options': [None, 'Relative cache stats', 'value', 'Cache', 'dnsdist.cache_rel', 'area'],
		'lines': [
			['cache-hits', 'hits', 'incremental'],
			['cache-misses', 'misses', 'incremental']
	]},
	'cache_abs': {
		'options': [None, 'Absolute cache stats', 'value', 'Cache', 'dnsdist.cache_abs', 'area'],
		'lines': [
			['cache-hits', 'hits', 'absolute'],
			['cache-misses', 'misses', 'absolute']
	]},
	'acl_abs': {
		'options': [None, 'AccessControlList stats', 'value', 'ACL', 'dnsdist.acl_abs', 'line'],
		'lines': [
			['acl-drops', 'drop by acl', 'absolute'],
			['rule-drop', 'drop by rule', 'absolute'],
			['rule-nxdomain', 'rnxdomain by rule', 'absolute'],
			['rule-refused', 'refused by rule', 'absolute']
	]},
	'noncompliant_abs': {
		'options': [None, 'Noncompliant data', 'value', 'Invalid', 'dnsdist.noncompliant_abs', 'line'],
		'lines': [
			['empty-queries', 'empty queries', 'absolute'],
		    ['no-policy', 'no policy', 'absolute'],
			['noncompliant-queries', 'noncompliant queries', 'absolute'],
			['noncompliant-responses', 'noncompliant responses', 'absolute']
	]},
	'queries_rel': {
		'options': [None, 'Relative query stats', 'value', 'Queries', 'dnsdist.queries_rel', 'line'],
		'lines': [
			['queries', 'queries', 'incremental'],
			['rdqueries', 'rd queries', 'incremental'],
			['responses', 'responses', 'incremental']
	]},
	'queries_abs': {
		'options': [None, 'Absolute query stats', 'value', 'Queries', 'dnsdist.queries_abs', 'line'],
		'lines': [
			['queries', 'queries', 'absolute'],
			['rdqueries', 'rd queries', 'absolute'],
			['responses', 'responses', 'absolute']
	]},
	'health_abs': {
		'options': [None, 'Health', 'value', 'Health', 'dnsdist.health_abs', 'line'],
		'lines': [
			['downstream-send-errors', 'ds send errors', 'absolute'],
			['downstream-timeouts', 'ds timeouts', 'absolute'],
			['servfail-responses', 'servfail responses', 'absolute'],
			['trunc-failures', 'trunc failures', 'absolute']
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
			
		rdict = dict()
		jdata = loads(data)
		for d in jdata:
			rdict[str(d)] = jdata[str(d)]
		
		return rdict

