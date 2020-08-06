
class TestState(object):
    def __init__(self, prefix="gaps_hi", network="_default"):
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


    def wipe(self):
        sh(f"docker-compose -f {base}/child-compose.yml -f {base}/middle-compose.yml -f {base}/parent-compose.yml down --remove-orphans", self.output)
        self.end_checks = []    # Before the containers are killed
        self.post_checks = []   # After the containers are killed (and the logs are final)
        self.nodes           = {}
        self.nodes['child']  = Node("child", self.prefix + "_agent_child_1", self.parser)
        self.nodes['middle'] = Node("middle", self.prefix + "_agent_middle_1", self.parser)
        self.nodes['parent'] = Node("parent", self.prefix + "_agent_parent_1", self.parser)

    def wrap(self, case):
        print(f"\n---------------> Wipe test state: {case.__name__}\n")
        with open(f"{case.__name__}.log","w") as f:
            self.output = f
            self.wipe()
            passed = True
            try:
                case(self)
            except Exception as e:
                passed = False
                print(f"{case.__name__} -> exception during test: {str(e)}")

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
        sh(f"docker-compose -f {base}/{node}-compose.yml up -d", self.output)
        container = json.loads(sh(f"docker inspect {self.nodes[node].container_name}",self.output))
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


    def check_sync(self, source, target):
        print(f"  check_sync {source} {target}", file=self.output)
        source_json = get_data(f"localhost:{self.nodes[source].port}", "system.cpu")
        if not source_json:
            print(f"  FAILED to check sync looking at http://localhost:{self.nodes[source].port}", file=self.output)
            return
        target_json = get_data(f"localhost:{self.nodes[source].port}", "system.cpu")
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
