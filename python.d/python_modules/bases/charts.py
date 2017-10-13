# -*- coding: utf-8 -*-
# Description:
# Author: Ilya Mashchenko (l2isbad)

from bases.collection import safe_print

CHART_PARAMS = ['type', 'id', 'name', 'title', 'units', 'family', 'context', 'chart_type']
DIMENSION_PARAMS = ['id', 'name', 'algorithm', 'multiplier', 'divisor']
CHART_TYPES = ['line', 'area', 'stacked']
DIMENSION_ALGORITHMS = ['absolute', 'incremental', 'percentage-of-absolute-row', 'percentage-of-incremental-row']

CHART_BEGIN = 'BEGIN {type}.{id} {since_last}\n'
CHART_CREATE = "CHART {type}.{id} '{name}' '{title}' '{units}' '{family}' '{context}' " \
               "{chart_type} {priority} {update_every}\n"
CHART_OBSOLETE = "CHART {type}.{id} '{name}' '{title}' '{units}' '{family}' '{context}' " \
               "{chart_type} {priority} {update_every} 'obsolete'\n"


DIMENSION_CREATE = "DIMENSION '{id}' '{name}' {algorithm} {multiplier} {divisor} '{hidden}'\n"
DIMENSION_SET = "SET '{id}' = {value}\n"

RUNTIME_CHART_CREATE = "CHART netdata.runtime_{job_name} '' 'Execution time for {job_name}' 'ms' 'python.d' " \
                       "netdata.pythond_runtime line 145000 {update_every}\n" \
                       "DIMENSION run_time 'run time' absolute 1 1\n"


def create_runtime_chart(func):
    """
    :param func: class method
    :return:
    """
    def wrapper(*args, **kwargs):
        self = args[0]
        ok = func(*args, **kwargs)
        if ok:
            safe_print(RUNTIME_CHART_CREATE.format(job_name=self.name,
                                                   update_every=self._runtime_counters.FREQ))
        return ok
    return wrapper


class ReAddingError(Exception):
    pass


class Checks:
    @staticmethod
    def chart_params(params, chart_name):
        """
        :param params: <list>
        :param chart_name: <str>
        :return:
        """
        for param in CHART_PARAMS:
            try:
                params[param]
            except KeyError:
                raise KeyError('{param} is missing ({chart})'.format(param=param, chart=chart_name))
            value = params[param]
            if any([param == 'id', param == 'units', param == 'context', param == 'chart_type']):
                if not value:
                    raise ValueError('Wrong chart value ({chart}). {param} must not be empty'.format(chart=chart_name,
                                                                                                     param=param))
            if param == 'chart_type':
                if value not in CHART_TYPES:
                    raise ValueError('Wrong chart type ({chart}). Must be on of {types}'.format(chart=chart_name,
                                                                                                types=CHART_TYPES))

    @staticmethod
    def dimension_params(params, chart_name):
        """
        :param params: <list>
        :param chart_name: <str>
        :return:
        """
        if not params.get('id'):
            raise ValueError('Wrong dimension id ({chart}). Must not be empty'.format(chart=chart_name))
        if params['algorithm'] not in DIMENSION_ALGORITHMS:
            raise ValueError('Wrong dimension algorithm ({chart}). '
                             'Must be one of {algorithms}'.format(chart=chart_name,
                                                                  algorithms=DIMENSION_ALGORITHMS))
        if not (str(params['multiplier']).isdigit() and str(params['divisor']).isdigit()):
            raise ValueError('Wrong multiplier or divisor ({chart}). Must be a digit'.format(chart=chart_name))


class Charts:
    def __init__(self, job_name, priority, update_every):
        """
        :param job_name: <bound method>
        :param priority: <int>
        :param update_every: <int>
        """
        self.job_name = job_name
        self.priority = priority
        self.update_every = update_every
        self.charts = dict()

    def __len__(self):
        return len(self.charts)

    def __iter__(self):
        return iter(self.charts.values())

    def __repr__(self):
        return str([chart for chart in self.charts])

    def __contains__(self, item):
        return item in self.charts

    def __getitem__(self, item):
        return self.charts[item]

    def __delitem__(self, key):
        del self.charts[key]

    def empty(self):
        return not bool(self.charts)

    def penalty_exceeded(self, penalty_max):
        """
        :param penalty_max: <int>
        :return:
        """
        return (chart for chart in self if chart.penalty > penalty_max)

    def add_chart(self, params):
        """
        :param params: <list>
        :return:
        """
        params = [self.job_name()] + params
        chart_id = params[1]
        if chart_id in self.charts:
            raise ReAddingError('{chart} already in charts'.format(chart=chart_id))
        else:
            new_chart = Chart(params)
            new_chart.params['update_every'] = self.update_every
            new_chart.params['priority'] = self.priority
            self.priority += 1
            self.charts[new_chart.params['id']] = new_chart
            return new_chart


class Chart:
    def __init__(self, params):
        """
        :param params: <list>
        """
        self.params = dict(zip(CHART_PARAMS, (p or str() for p in params)))
        self.name = '.'.join([self.params['type'], self.params['id']])
        Checks.chart_params(self.params, self.name)
        self.dimensions = list()
        self.penalty = 0

    def __repr__(self):
        return str(self.params)

    def __str__(self):
        return self.params['id']

    def add_dimension(self, dimension):
        """
        :param dimension: <list>
        :return:
        """
        if dimension[0] in self.dimensions:
            raise ReAddingError('{dimension} already in {chart} dimensions'.format(dimension=dimension[0],
                                                                                   chart=self.name))
        self.dimensions.append(Dimension(dimension, self.name))

    def add_dimension_and_push_chart(self, dimension):
        """
        :param dimension: <list>
        :return:
        """
        if dimension[0] in self.dimensions:
            raise ReAddingError('{dimension} already in {chart} dimensions'.format(dimension=dimension[0],
                                                                                   chart=self.name))
        dimension = Dimension(dimension, self.name)
        self.dimensions.append(dimension)
        safe_print(self.create(dimension))

    def create(self, dimension=None):
        """
        :param dimension: <list>
        :return:
        """
        chart = CHART_CREATE.format(**self.params)
        if not dimension:
            dimensions = ''.join([dimension.create() for dimension in self.dimensions])
        else:
            dimensions = dimension.create()
        return chart + dimensions

    def begin(self, since_last):
        """
        :param since_last: <int>: microseconds
        :return:
        """
        return CHART_BEGIN.format(type=self.params['type'],
                                  id=self.params['id'],
                                  since_last=since_last)

    def obsolete(self):
        return CHART_OBSOLETE.format(**self.params)


class Dimension:
    def __init__(self, params, chart_name):
        """
        :param params: <list>
        :param chart_name: <str>
        """
        self.params = dict(zip(DIMENSION_PARAMS, (p or str() for p in params)))
        self.params['name'] = self.params.get('name') or self.params.get('id')
        self.params.setdefault('algorithm', 'incremental')
        self.params.setdefault('multiplier', 1)
        self.params.setdefault('divisor', 1)
        self.params.setdefault('hidden', '')
        Checks.dimension_params(self.params, chart_name)

    def __repr__(self):
        return self.params['id']

    def create(self):
        return DIMENSION_CREATE.format(**self.params)

    def set(self, value):
        """
        :param value: <str>: must be a digit
        :return:
        """
        return DIMENSION_SET.format(id=self.params['id'],
                                    value=value)
