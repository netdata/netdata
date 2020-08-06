import json, os, os.path, re, requests, shutil, time, traceback
from AgentTest.misc import sh
from AgentTest.Node import Node

def compare_data(source, replica, output, max_pre=0, max_post=0):
    passed = True
    source_times = [ x[0] for x in source ]
    source_start = min(source_times)
    source_end   = max(source_times)
    source_by_time = dict([(d[0],d[1:]) for d in source])

    replica_times = [ x[0] for x in replica ]
    replica_start = min(replica_times)
    replica_end   = max(replica_times)
    replica_by_time = dict([(d[0],d[1:]) for d in replica])

    if abs(replica_start-source_start)>max_pre:
        print(f"Mismatch in start times {source_start} -> {replica_start})", file=output)
        passed = False
    if abs(replica_end-source_end)>max_post:
        print(f"Mismatch in start times {source_start} -> {replica_start})", file=output)
        passed = False

    if replica_start < source_start or replica_end > replica_end:
        print("Garbage in replica", file=output)
        passed = False

    common_start = max(source_start,replica_start)
    common_end  = min(source_end,replica_end)

    for t in range(common_start, common_end+1):
        if not t in replica_by_time:
            print(f"{t} is missing from replica!", file=output)
            passed = False
        if source_by_time[t] != replica_by_time[t]:
            print(f"@{t} {source_by_time[t]} != {replica_by_time[t]})", file=output)
            passed = False
    return passed

class LogParser(object):
    def __init__(self, events):
        self.events = []
        self.matchers = dict([(name,re.compile(regex)) for (name,regex) in events.items()])

    def parse(self, filename):
        self.lines = open(filename).readlines()
        self.events = [f"{self.lines[0][:19]} start"]
        for line in self.lines:
            for n,r in self.matchers.items():
                if r.search(line):
                    self.events.append(f"{line[:19]} {n}")
        self.events.append(f"{self.lines[-1][:19]} end")
        return sorted(set(self.events))

class State(object):
    def __init__(self, working, prefix="agent_test", network="_default"):
        self.working         = working
        if not os.path.isdir(working):
            os.mkdir(working)
        self.network         = prefix + network
        self.prefix          = prefix
        self.nodes           = {}
        self.output          = None
        self.parser          = LogParser({ "child connect": "client willing",
                                           "child disconnect" : "STREAM child.*disconnected \(completed",
                                           "agent start": "Enabling reaper",
                                           "connect failed (DNS)" : "Cannot resolve host",
                                           "connect failed (port closed)" : "connection refused",
                                           "gap detect"   : "Gap detect",
                                           "data rx"      : "RECEIVER",
                                           "data tx"      : "STREAM: Sending data. Buffer",
                                           "replication"  : "REPLIC" })
        # Suppress DNS failures in two node scenario on the top level
        self.parser2         = LogParser({ "child connect": "client willing",
                                           "child disconnect" : "STREAM child.*disconnected \(completed",
                                           "agent start": "Enabling reaper",
                                           "connect failed (port closed)" : "connection refused",
                                           "gap detect"   : "Gap detect",
                                           "data rx"      : "RECEIVER",
                                           "data tx"      : "STREAM: Sending data. Buffer",
                                           "replication"  : "REPLIC" })

    def add_node(self, name):
        n = Node(name, f"{self.prefix}_{name}_1", self.parser)
        self.nodes[name] = n
        return n

    def wrap(self, case):
        # Clean any old data from working
        self.test_base = os.path.join(self.working, case.__name__)
        if os.path.isdir(self.test_base):
            shutil.rmtree(self.test_base)
        os.mkdir(self.test_base)

        print(f"\n---------------> Wipe test state: {case.__name__}\n")
        with open(os.path.join(self.test_base,"test.log"),"w") as f:
            self.output = f
            # Setup initial node state and generate config
            for n in self.nodes.values():
                n.create_config(self.test_base)
            self.end_checks = []    # Before the containers are killed
            self.post_checks = []   # After the containers are killed (and the logs are final)

            composes = " ".join([f"-f {self.test_base}/{n.name}-compose.yml" for n in self.nodes.values()])
            sh(f"docker-compose -p {self.prefix} {composes} down --remove-orphans", self.output)
            passed = True
            try:
                case(self)
            except Exception as e:
                passed = False
                print(f"{case.__name__} -> exception during test: {str(e)}")
                traceback.print_tb(e)

            for c in self.end_checks:
                passed = c() and passed         # Shortcut logic, left to right
            for n in self.nodes.values():
                if n.started:
                    sh(f"docker kill {n.container_name}", output=f)
                    n.log = f"{case.__name__}-{n.name}.log"
                    sh(f"docker logs {n.container_name} >{n.log} 2>&1", output=f)
            for c in self.post_checks:
                passed = c() and passed         # Shortcut logic, left to right
            for n in self.nodes.values():
                if n.started:
                    ev = n.parser.parse(n.log)
                    for e in ev:
                        print(n.name, e, file=f)
            print(f"{case.__name__} -> {passed}")
            print(f"{case.__name__} -> {passed}", file=f)
            self.output = None


    def start(self, node):
        sh(f"docker-compose -p {self.prefix} -f {self.test_base}/{node}-compose.yml up -d", self.output)
        container = json.loads(sh(f"docker inspect {self.nodes[node].container_name}",self.output))
        # This catches dynamically assigned ports but auto-scaling is poor in Docker so we normally set these
        # in the test description.
        self.nodes[node].port = container[0]["NetworkSettings"]["Ports"]["19999/tcp"][0]["HostPort"]
        self.nodes[node].started = True

    def wait_up(self, node):
        url = f"http://localhost:{self.nodes[node].port}/api/v1/info"
        print(f"  Waiting for {node} on {url}", file=self.output)
        while True:
            try:
                r = requests.get(url)
                info = requests.get(url).json()
                self.nodes[node].guid = info['uid']
                return
            except requests.ConnectionError:
                print(f"  Waiting for {node}...", file=self.output)
            except json.decoder.JSONDecodeError:
                print(f"  Waiting for {node}...", file=self.output)
            time.sleep(1)

    def wait_connected(self, sender, receiver):
        '''This will detect the *first time* connection of a child to a parent. It looks in the mirrored
           hosts array so on reconnections it will return instantly because the child database already
           exists on the parent.'''
        url = f"http://localhost:{self.nodes[receiver].port}/api/v1/info"
        print(f"  Waiting for {sender} to connect to {receiver}", file=self.output)
        while True:
            try:
                r = requests.get(url)
                info = requests.get(url).json()
            except requests.ConnectionError:
                print(f"  {receiver} not responding...", file=self.output)
                time.sleep(1)
                continue
            if sender in info['mirrored_hosts']:
               print(f"  {sender} in mirrored_hosts on {receiver}", file=self.output)
               return
            print(f"  {receiver} mirrors {info['mirrored_hosts']}...", file=self.output)
            time.sleep(1)

    def wait_isparent(self, node):
        '''This will detect the connection of some child to a parent. It cannot check which one connected
           in scenarios with multiple children of a parent.'''
        url = f"http://localhost:{self.nodes[node].port}/api/v1/info"
        print(f"  Waiting for {node} to become parent", file=self.output)
        attempts = 0
        while attempts < 30:
            try:
                attempts += 1
                r = requests.get(url)
                info = requests.get(url).json()
            except json.decoder.JSONDecodeError:
                print(f"  {node} returned empty...", file=self.output)
                time.sleep(1)
                continue
            except requests.ConnectionError:
                print(f"  {node} not responding...", file=self.output)
                time.sleep(1)
                continue
            if info['host_labels']['_is_parent'] == 'true':
               print(f"  {node} has child connected", file=self.output)
               return
            print(f"  {node} has host labels {info['host_labels']}...", file=self.output)
            time.sleep(1)
        raise Exception(f"Node {node} did not become parent when expected")

    def check_norep(self):
        '''Check that replication did not occur during the test by scanning the logs for debug.'''
        failed = False
        for n in self.nodes.values():
            if n.started and len(sh(f"grep -i replic {n.log}",self.output))>0:
                print(f"  FAILED {n.name} was involved in replication", file=self.output)
                failed = True
        if not failed:
            print(f"  PASSED no replication detected on {n.name}", file=self.output)
        return not failed

    def check_rep(self):
        '''Check that replication did occur during the test by scanning the logs for debug.'''
        for n in self.nodes.values():
            if n.started and len(sh(f"grep -i replic {n.log}",self.output))>0:
                print(f"  PASSED {n.name} was involved in replication", file=self.output)
                return True
        print(f"  FAILED no replication detected on nodes", file=self.output)
        return False


    # TODO: Check on a larger set of charts, include disk.io
    def check_sync(self, source, target):
        print(f"  check_sync {source} {target}", file=self.output)
        source_json = self.nodes[source].get_data("system.cpu")
        if not source_json:
            print(f"  FAILED to check sync looking at http://localhost:{self.nodes[source].port}", file=self.output)
            return
        target_json = self.nodes[target].get_data("system.cpu")
        if not target_json:
            print(f"  FAILED to check sync looking at http://localhost:{self.nodes[target].port}", file=self.output)
            return
        if source_json["labels"] != target_json["labels"]:
            print(f"  Mismatch in chart labels: source={source_json['labels']} target={target_json['labels']}", file=self.output)
        source_data = source_json["data"]
        target_data = target_json["data"]

        if compare_data(source_data, target_data, self.output):
            print("  PASSED in compare", file=self.output)
            return True
        else:
            print("  FAILED in compare", file=self.output)
            print(source_data, file=self.output)
            return False
