import json, os.path, requests

class Node(object):
    def __init__(self, name, cname, parser):
        self.name = name
        self.container_name = cname
        self.port = None        # Exposed on host, default is not to expose
        self.guid = None        # Randomly generate
        self.log = None
        self.started = False
        self.parser = parser
        self.stream_target = None
        self.receiver = False
        self.db_mode = "dbengine"

    def stream_to(self, target):
        self.stream_target = target
        target.receiver = True

    def create_config(self, base):
        compose = os.path.join(base, f"{self.name}-compose.yml")
        guid = os.path.join(base, f"{self.name}-guid")
        conf = os.path.join(base, f"{self.name}-netdata.conf")
        stream = os.path.join(base, f"{self.name}-stream.conf")
        with open(guid, "w") as f:
            print(self.guid,file=f)
        with open(compose, "w") as f:
            print(f"version: '3.3'", file=f)
            print(f"services:", file=f)
            print(f"    {self.name}:", file=f)
            print(f"        image: debian_10_dev", file=f)
            print(f"        command: /usr/sbin/netdata -D", file=f)
            if self.port is not None:
                print(f"        ports:", file=f)
                print(f"            - {self.port}:19999", file=f)
            print(f"        volumes:", file=f)
            print(f"            - {stream}:/etc/netdata/stream.conf:ro", file=f)
            print(f"            - {guid}:/var/lib/netdata/registry/netdata.public.unique.id:ro", file=f)
            print(f"            - {conf}:/etc/netdata/netdata.conf:ro", file=f)
            print(f"        cap_add:", file=f)
            print(f"            - SYS_PTRACE", file=f)
        with open(conf, "w") as f:
            print(f"[global]", file=f)
            print(f"    debug flags = 0x00000000c0000000", file=f)
            print(f"    errors flood protection period = 0", file=f)
            print(f"    hostname = {self.name}", file=f)
            print(f"    memory mode = {self.db_mode}", file=f)
            print(f"[web]", file=f)
            print(f"    ssl key = /etc/netdata/ssl/key.pem", file=f)
            print(f"    ssl certificate = /etc/netdata/ssl/cert.pem", file=f)
        with open(stream, "w") as f:
            if self.stream_target is not None:
                print(f"[stream]", file=f)
                print(f"    enabled = yes", file=f)
                print(f"    destination = tcp:{self.stream_target.name}", file=f)
                print(f"    api key = 00000000-0000-0000-0000-000000000000", file=f)
                print(f"    timeout seconds = 60", file=f)
                print(f"    default port = 19999", file=f)
                print(f"    send charts matching = *", file=f)
                print(f"    buffer size bytes = 10485760", file=f)
                print(f"    reconnect delay seconds = 5", file=f)
                print(f"    initial clock resync iterations = 60", file=f)
            if self.receiver:
                print(f"[00000000-0000-0000-0000-000000000000]", file=f)
                print(f"    enabled = yes", file=f)
                print(f"    allow from = *", file=f)
                print(f"    default history = 3600", file=f)
                print(f"    # default memory mode = ram", file=f)
                print(f"    health enabled by default = auto", file=f)
                print(f"    # postpone alarms for a short period after the sender is connected", file=f)
                print(f"    default postpone alarms on connect seconds = 60", file=f)
                print(f"    multiple connections = allow", file=f)

    def get_data(self, chart, host=None):
        if host is None:
            url = f"http://localhost:{self.port}/api/v1/data?chart={chart}"
        else:
            url = f"http://localhost:{self.port}/host/{host}/api/v1/data?chart={chart}"
        try:
            r = requests.get(url)
            return r.json()
        except json.decoder.JSONDecodeError:
            print(f"  Fetch failed {url} -> {r.text}")
            return None
        except requests.exceptions.ConnectionError:
            print(f"  Fetch failed {url} -> connection refused")
            return None

    def get_charts(self):
        url = f"http://localhost:{self.port}/api/v1/charts"
        try:
            r = requests.get(url)
            return r.json()
        except json.decoder.JSONDecodeError:
            print(f"  Fetch failed {url} -> {r.text}")
            return None
        except requests.exceptions.ConnectionError:
            print(f"  Fetch failed {url} -> connection refused")
            return None
