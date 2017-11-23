# -*- coding: utf-8 -*-
from json import loads
from bases.FrameworkServices.UrlService import UrlService

ORDER = ['queries', 'queries_dropped', 'answers', 'backend_responses', 'backend_commerrors', 'backend_errors', 'cache', 'servercpu', 'servermem', 'query_latency', 'query_latency_avg']
CHARTS = {
	'queries': {
		'options': [None, 'Client queries received', 'queries/s', 'queries', 'dnsdist.queries', 'line'],
		'lines': [
			['queries', 'all', 'incremental'],
			['rdqueries', 'recursive', 'incremental'],
			['empty-queries', 'empty', 'incremental']
	]},
	'queries_dropped': {
		'options': [None, 'Client queries dropped', 'queries/s', 'queries', 'dnsdist.queries_dropped', 'line'],
		'lines': [
			['rule-drop', 'rule drop', 'incremental'],
			['dyn-blocked', 'dynamic block', 'incremental'],
			['no-policy', 'no policy', 'incremental'],
			['noncompliant-queries', 'non compliant', 'incremental'],
			['acl-drops', 'acl', 'incremental'],
	]},
	'answers': {
		'options': [None, 'Answers statistics', 'answers/s', 'answers', 'dnsdist.answers', 'line'],
		'lines': [
			['self-answered', 'self answered', 'incremental'],
			['rule-nxdomain', 'nxdomain', 'incremental', -1],
			['rule-refused', 'refused', 'incremental', -1],
			['trunc-failures', 'trunc failures', 'incremental', -1]
	]},
	'backend_responses': {
		'options': [None, 'Backend responses', 'responses/s', 'backends', 'dnsdist.backend_responses', 'line'],
		'lines': [
			['responses', 'responses', 'incremental']
	]},
	'backend_commerrors': {
		'options': [None, 'Backend Communication Errors', 'errors/s', 'backends', 'dnsdist.backend_commerrors', 'line'],
		'lines': [
			['downstream-send-errors', 'send errors', 'incremental']
	]},
	'backend_errors': {
		'options': [None, 'Backend error responses', 'responses/s', 'backends', 'dnsdist.backend_errors', 'line'],
		'lines': [
			['downstream-timeouts', 'timeout', 'incremental'],
			['servfail-responses', 'servfail', 'incremental'],
			['noncompliant-responses', 'non compliant', 'incremental']
	]},
	'cache': {
		'options': [None, 'Cache performance', 'answers/s', 'cache', 'dnsdist.cache', 'area'],
		'lines': [
			['cache-hits', 'hits', 'incremental'],
			['cache-misses', 'misses', 'incremental', -1]
	]},
	'servercpu': {
		'options': [None, 'DNSDIST server CPU utilization', 'ms/s', 'server', 'dnsdist.servercpu', 'stacked'],
		'lines': [
			['cpu-sys-msec', 'system state', 'incremental'],
			['cpu-user-msec', 'user state', 'incremental']
	]},
	'servermem': {
		'options': [None, 'DNSDIST server memory utilization', 'MiB', 'server', 'dnsdist.servermem', 'area'],
		'lines': [
			['real-memory-usage', 'memory usage', 'absolute', 1, 1048576]
	]},
	'query_latency': {
		'options': [None, 'Query latency', 'queries/s', 'latency', 'dnsdist.query_latency', 'stacked'],
		'lines': [
			['latency0-1', '1ms', 'incremental'],
			['latency1-10', '10ms', 'incremental'],
			['latency10-50', '50ms', 'incremental'],
			['latency50-100', '100ms', 'incremental'],
			['latency100-1000', '1sec', 'incremental'],
			['latency-slow', 'slow', 'incremental']
	]},
	'query_latency_avg': {
		'options': [None, 'Average latency for the last N queries', 'ms/query', 'latency', 'dnsdist.query_latency_avg', 'line'],
		'lines': [
			['latency-avg100', '100', 'absolute'],
			['latency-avg1000', '1k', 'absolute'],
			['latency-avg10000', '10k', 'absolute'],
			['latency-avg1000000', '1000k', 'absolute']
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

