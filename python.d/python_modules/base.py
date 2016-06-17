# Description: base for netdata python.d plugins
# Author: Pawel Krupa (paulfantom)

class BaseService(object):
    def __init__(self,configuration=None,update_every=None,priority=None,retries=None):
        if None in (configuration,update_every,priority,retries):
            # use defaults
            self.error("BaseService: no configuration parameters supplied. Cannot create Service.")
            raise RuntimeError
        else:
            self._parse_base_config(configuration,update_every,priority,retries)

    def _parse_base_config(self,config,update_every,priority,retries):
        # parse configuration options to run this Service
        try:
            self.update_every = int(config['update_every'])
        except (KeyError, ValueError):
            self.update_every = update_every
        try:
            self.priority = int(config['priority'])
        except (KeyError, ValueError):
            self.priority = priority
        try:
            self.retries = int(config['retries'])
        except (KeyError, ValueError):
            self.retries = retries
        self.retries_left = self.retries

    def error(self, msg, exception=""):
        if exception != "":
            exception = " " + str(exception).replace("\n"," ")
        sys.stderr.write(str(msg)+exception+"\n")
        sys.stderr.flush()

    def check(self):
        # TODO notify about not overriden function
        self.error("Where is your check()?")
        return False

    def create(self):
        # TODO notify about not overriden function
        self.error("Where is your create()?")
        return False

    def update(self):
        # TODO notify about not overriden function
        self.error("Where is your update()?")
        return False
