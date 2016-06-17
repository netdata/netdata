# Description: base for netdata python.d plugins
# Author: Pawel Krupa (paulfantom)

class BaseService(object):
    def __init__(self,configuration,update_every,priority,retries):
        if configuration is None:
            # use defaults
            configuration = config
            self.error(NAME+": no configuration supplied. using defaults.")

        self._parse_base_config(configuration)

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
