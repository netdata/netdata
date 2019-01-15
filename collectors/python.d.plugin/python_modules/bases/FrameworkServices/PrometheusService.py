from .UrlService import UrlService

try:
    from prometheus_client.parser import text_fd_to_metric_families
    HAS_PROMETHEUS = True
except ImportError:
    HAS_PROMETHEUS = False


def get_metric_by_name(metrics, name):
    for metric in metrics:
        if metric.name == name:
            return metric
    return None


class PrometheusService(UrlService):

    def _get_raw_data(self, url=None, manager=None):
        raw = UrlService._get_raw_data(self, url=None, manager=None)

        if not raw:
            return None

        lines = iter(raw.split('\n'))

        try:
            metrics = [m for m in text_fd_to_metric_families(lines)]
        except AttributeError as error:
            self.error('parse error: {0}'.format(error))
            return None

        return metrics

    def check(self):
        if not HAS_PROMETHEUS:
            self.error('python-prometheus_client package is needed to use PrometheusService')
            return False
        return UrlService.check(self)
