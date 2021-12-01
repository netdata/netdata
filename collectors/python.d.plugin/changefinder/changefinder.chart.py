# -*- coding: utf-8 -*-
# Description: changefinder netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads
import re

from bases.FrameworkServices.UrlService import UrlService

import numpy as np
import changefinder
from scipy.stats import percentileofscore

update_every = 5
disabled_by_default = True

ORDER = [
    'scores',
    'flags'
]

CHARTS = {
    'scores': {
        'options': [None, 'ChangeFinder', 'score', 'Scores', 'scores', 'line'],
        'lines': []
    },
    'flags': {
        'options': [None, 'ChangeFinder', 'flag', 'Flags', 'flags', 'stacked'],
        'lines': []
    }
}

DEFAULT_PROTOCOL = 'http'
DEFAULT_HOST = '127.0.0.1:19999'
DEFAULT_CHARTS_REGEX = 'system.*'
DEFAULT_MODE = 'per_chart'
DEFAULT_CF_R = 0.5
DEFAULT_CF_ORDER = 1
DEFAULT_CF_SMOOTH = 15
DEFAULT_CF_DIFF = False
DEFAULT_CF_THRESHOLD = 99
DEFAULT_N_SCORE_SAMPLES = 14400
DEFAULT_SHOW_SCORES = False


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.protocol = self.configuration.get('protocol', DEFAULT_PROTOCOL)
        self.host = self.configuration.get('host', DEFAULT_HOST)
        self.url = '{}://{}/api/v1/allmetrics?format=json'.format(self.protocol, self.host)
        self.charts_regex = re.compile(self.configuration.get('charts_regex', DEFAULT_CHARTS_REGEX))
        self.charts_to_exclude = self.configuration.get('charts_to_exclude', '').split(',')
        self.mode = self.configuration.get('mode', DEFAULT_MODE)
        self.n_score_samples = int(self.configuration.get('n_score_samples', DEFAULT_N_SCORE_SAMPLES))
        self.show_scores = int(self.configuration.get('show_scores', DEFAULT_SHOW_SCORES))
        self.cf_r = float(self.configuration.get('cf_r', DEFAULT_CF_R))
        self.cf_order = int(self.configuration.get('cf_order', DEFAULT_CF_ORDER))
        self.cf_smooth = int(self.configuration.get('cf_smooth', DEFAULT_CF_SMOOTH))
        self.cf_diff = bool(self.configuration.get('cf_diff', DEFAULT_CF_DIFF))
        self.cf_threshold = float(self.configuration.get('cf_threshold', DEFAULT_CF_THRESHOLD))
        self.collected_dims = {'scores': set(), 'flags': set()}
        self.models = {}
        self.x_latest = {}
        self.scores_latest = {}
        self.scores_samples = {}

    def get_score(self, x, model):
        """Update the score for the model based on most recent data, flag if it's percentile passes self.cf_threshold.
        """

        # get score
        if model not in self.models:
            # initialise empty model if needed
            self.models[model] = changefinder.ChangeFinder(r=self.cf_r, order=self.cf_order, smooth=self.cf_smooth)
        # if the update for this step fails then just fallback to last known score
        try:
            score = self.models[model].update(x)
            self.scores_latest[model] = score
        except Exception as _:
            score = self.scores_latest.get(model, 0)
        score = 0 if np.isnan(score) else score

        # update sample scores used to calculate percentiles
        if model in self.scores_samples:
            self.scores_samples[model].append(score)
        else:
            self.scores_samples[model] = [score]
        self.scores_samples[model] = self.scores_samples[model][-self.n_score_samples:]

        # convert score to percentile
        score = percentileofscore(self.scores_samples[model], score)

        # flag based on score percentile
        flag = 1 if score >= self.cf_threshold else 0

        return score, flag

    def validate_charts(self, chart, data, algorithm='absolute', multiplier=1, divisor=1):
        """If dimension not in chart then add it.
        """
        if not self.charts:
            return

        for dim in data:
            if dim not in self.collected_dims[chart]:
                self.collected_dims[chart].add(dim)
                self.charts[chart].add_dimension([dim, dim, algorithm, multiplier, divisor])

        for dim in list(self.collected_dims[chart]):
            if dim not in data:
                self.collected_dims[chart].remove(dim)
                self.charts[chart].del_dimension(dim, hide=False)

    def diff(self, x, model):
        """Take difference of data.
        """
        x_diff = x - self.x_latest.get(model, 0)
        self.x_latest[model] = x
        x = x_diff
        return x

    def _get_data(self):

        # pull data from self.url
        raw_data = self._get_raw_data()
        if raw_data is None:
            return None

        raw_data = loads(raw_data)

        # filter to just the data for the charts specified
        charts_in_scope = list(filter(self.charts_regex.match, raw_data.keys()))
        charts_in_scope = [c for c in charts_in_scope if c not in self.charts_to_exclude]

        data_score = {}
        data_flag = {}

        # process each chart
        for chart in charts_in_scope:

            if self.mode == 'per_chart':

                # average dims on chart and run changefinder on that average
                x = [raw_data[chart]['dimensions'][dim]['value'] for dim in raw_data[chart]['dimensions']]
                x = [x for x in x if x is not None]

                if len(x) > 0:

                    x = sum(x) / len(x)
                    x = self.diff(x, chart) if self.cf_diff else x

                    score, flag = self.get_score(x, chart)
                    if self.show_scores:
                        data_score['{}_score'.format(chart)] = score * 100
                    data_flag[chart] = flag

            else:

                # run changefinder on each individual dim
                for dim in raw_data[chart]['dimensions']:

                    chart_dim = '{}|{}'.format(chart, dim)

                    x = raw_data[chart]['dimensions'][dim]['value']
                    x = x if x else 0
                    x = self.diff(x, chart_dim) if self.cf_diff else x

                    score, flag = self.get_score(x, chart_dim)
                    if self.show_scores:
                        data_score['{}_score'.format(chart_dim)] = score * 100
                    data_flag[chart_dim] = flag

        self.validate_charts('flags', data_flag)

        if self.show_scores & len(data_score) > 0:
            data_score['average_score'] = sum(data_score.values()) / len(data_score)
            self.validate_charts('scores', data_score, divisor=100)

        data = {**data_score, **data_flag}

        return data
