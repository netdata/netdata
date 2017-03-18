from base import LogService

priority = 60000
retries = 60
# update_every = 3

ORDER = ['intel_gpu']
CHARTS = {
    'intel_gpu': {
        'options': [None, 'Intel GPU usage', 'percent', 'usage', 'intel_gpu.usage', 'line'],
        'lines': [
            ["render"],
            ["bitstream"],
            ["blitter"]
        ]
    }
}

class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        if len(self.log_path) == 0:
            self.log_path = "/var/log/intel_gpu_top.log"
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        try:
            raw = self._get_raw_data()
            if raw is None:
                return None
            elif not raw:
                return {'render': 0,
                        'bitstream': 0,
                        'blitter': 0}

        except (ValueError, AttributeError):
            return None
            
        row = raw[-1].split()

        if row[0] == "#":
            return None

        ret = {'render': row[1],
               'bitstream': row[3] if row[3] != "-1" else row[5]}

        if row[7] != "-1":
            ret['blitter'] = row[7]

        return ret
