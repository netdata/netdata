import json, os, os.path, re, requests, shutil, time, traceback
from AgentTest.misc import sh
from AgentTest.Node import Node

class DSlice(object):
    def __init__(self, raw_json, skew):
        try:
            data = sorted(raw_json["data"])   # Timestamps ascending
            self.name = "+".join(raw_json["labels"][1:])
            self.start = data[0][0] + skew
            self.skew = skew
            # Remove sparseness when update_every>1, fill empty slots with None
            self.points = []
            self.blank = [None] * (len(data[0])-1)
            next_t = self.start
            for d in data:
                if d[0]+skew != next_t:
                    self.points.extend( [self.blank]*(d[0]+skew-next_t) )
                self.points.append( d[1:] )
                next_t = d[0]+skew+1
        except:
            print(f"Invalid json data: {raw_json}")
            raise Exception("failed")

    def __str__(self):
        return f"{self.name}@{self.start}={self.points}"

    def score(self, other):
        self_end = self.start + len(self.points)
        other_end = other.start + len(other.points)
        common_start = max(self.start, other.start)
        common_end  = min(self_end, other_end)

        self_in_common = self.points[common_start-self.start:common_end-self.start]
        other_in_common = other.points[common_start-other.start:common_end-other.start]

        count = 0
        for s,o in zip(self_in_common, other_in_common):
            if s!=o:
                count += 1

        return count, abs(other.start-self.start), abs(other_end-self_end)

    def get(self, timestamp):
        if timestamp < self.start or timestamp >= self.start + len(self.points):
            return self.blank
        return self.points[ timestamp - self.start ]

def show_mismatch(source, target, output):
    end = max(source.start + len(source.points), target.start + len(target.points))
    print(f"  source {source.start}-{source.start+len(source.points)} target {target.start}-{target.start + len(target.points)}", file=output)
    for t in range(source.start, end+1):
        source_sample = source.get(t)
        target_sample = target.get(t)
        if source_sample != target_sample:
            print(f"  {source.name}@{t} source {source_sample} target {target_sample}", file=output)


def cmp_dimension(source, target, max_pre=0, max_post=0):
    msgs = []
    source_end = source.start + len(source.points)
    target_end = target.start + len(target.points)

    common_start = max(source.start, target.start)
    common_end  = min(source_end,target_end)

    passed = True
    if abs(target.start-source.start)>max_pre:
        msgs.append(f"  Mismatch in start times {source.start} -> {target.start} ({abs(target.start-source.start)} > max_pre={max_pre})")
        passed = False
    if abs(target_end-source_end)>max_post:
        msgs.append(f"  Mismatch in end times {source_end} -> {target_end} ({abs(target_end-source_end)} > max_post={max_post})")
        passed = False

    source_in_common = source.points[common_start-source.start:common_end-source.start]
    target_in_common = target.points[common_start-target.start:common_end-target.start]

    if source_in_common != target_in_common:
        passed = False
        msgs.append(f"  Mismatch in {source.name} between {common_start} and {common_end}")
        msgs.append(f"  source: {source}")
        msgs.append(f"  target: {target}")
    return passed, msgs


def compare_data(source, replica, output, max_pre=0, max_post=0, skew=0):
    passed = True
    source_times = [ x[0]-skew for x in source ]
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

    if replica_start < source_start or replica_end > source_end:
        print("Garbage in replica", file=output)
        passed = False

    common_start = max(source_start,replica_start)
    common_end  = min(source_end,replica_end)

    for t in range(common_start, common_end+1):
        if not t in source_by_time:
            continue      # Range won't be dense if update_every>1 or child restarted
        if not (t+skew) in replica_by_time:
            print(f"{t} is missing from replica (skew={skew})!", file=output)
            passed = False
        if source_by_time[t] != replica_by_time[t+skew]:
            print(f"@{t} skew={skew} {source_by_time[t]} != {replica_by_time[t+skew]}", file=output)
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
    def __init__(self, working, config, config_label, prefix="agent_test", network="_default"):
        self.working         = working
        if not os.path.isdir(working):
            os.mkdir(working)
        self.config    = config                     # Used in directory name
        self.config_label    = config_label         # Human readable
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
        self.test_base = os.path.join(self.working, f"{case.__name__}_{self.config}")
        if os.path.isdir(self.test_base):
            shutil.rmtree(self.test_base)
        os.mkdir(self.test_base)

        print(f"\n---------------> Wipe test state: {case.__name__} in {self.config_label}\n")
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
                    n.log = os.path.join(self.test_base, f"{case.__name__}-{n.name}.log")
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

    def stop_net(self, node):
        sh(f"docker network disconnect {self.network} {self.nodes[node].container_name}", self.output)

    def start_net(self, node):
        sh(f"docker network connect --alias {node} {self.network} {self.nodes[node].container_name}", self.output)

    def kill(self, node):
        sh(f"docker kill -s INT {self.nodes[node].container_name}", self.output)

    def restart(self, node):
        sh(f"docker start {self.nodes[node].container_name}", self.output)

    def wait_up(self, node):
        url = f"http://localhost:{self.nodes[node].port}/api/v1/info"
        print(f"  Waiting for {node} on {url}", file=self.output)
        while True:
            try:
                r = requests.get(url)
                info = r.json()
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
                info = r.json()
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
                info = r.json()
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


    def check_sync(self, source, target, max_pre=0, max_post=0):
        if self.nodes[source].stream_target != self.nodes[target]:
            print(f"  TEST ERROR cannot check sync as {source} does not stream to {target}")
            return False
        chart_info = self.nodes[source].get_charts()
        if not chart_info:
            print(f"  FAILED to retrieve charts from child")
            return False
        charts = ("system.cpu", "system.load", "system.io", "system.ram", "system.ip", "system.processes")
        passed = True
        for ch in charts:
            update_every = chart_info["charts"][ch]["update_every"]
            print(f"  check_sync {source} {target} {ch} {update_every}", file=self.output)

            source_json = self.nodes[source].get_data(ch)
            if not source_json:
                print(f"  FAILED to check sync looking at http://localhost:{self.nodes[source].port}", file=self.output)
                passed = False
                continue
            if len(source_json["data"])==0:
                print(f"  FAILED to retrieve {ch} from {source} - response has zero rows", file=self.output)
                passed = False
                continue
            with open(os.path.join(self.test_base,f"{source}-{ch}.json"),"wt") as f:
                f.write(json.dumps(source_json, sort_keys=True, indent=4))
            target_json = self.nodes[target].get_data(ch,host=source)
            if not target_json:
                print(f"  FAILED to check sync looking at http://localhost:{self.nodes[target].port}", file=self.output)
                passed = False
                continue
            if len(target_json["data"])==0:
                print(f"  FAILED to retrieve {ch} from {target} - response has zero rows", file=self.output)
                passed = False
                continue
            if source_json["labels"] != target_json["labels"]:
                print(f"  Mismatch in chart labels on {ch}: source={source_json['labels']} target={target_json['labels']}", file=self.output)
            with open(os.path.join(self.test_base,f"{target}-{ch}.json"),"wt") as f:
                f.write(json.dumps(target_json, sort_keys=True, indent=4))

            source_sl = DSlice(source_json,0)
            best_match, best_score = None, None
            for skew in (-2*update_every, -update_every, 0, update_every, 2*update_every):
                target_sl = DSlice(target_json,skew)
                score, _, post = source_sl.score(target_sl)
                #print(f"  skew={skew} score={score} pre={pre} post={post}")
                #show_mismatch(source_sl, target_sl)
                if best_match is None or score < best_score:
                    best_match, best_score = target_sl, score

            if best_score == 0:
                print(f"  Data match {ch} with skew={best_match.skew}", file=self.output)
            else:
                print(f"  Data mismatch {ch}, closest with skew={best_match.skew}", file=self.output)
                show_mismatch(source_sl, best_match, self.output)
                passed = False


            #source_data = source_json["data"]
            #target_data = target_json["data"]

            #if compare_data(source_data, target_data, self.output, max_pre=max_pre, max_post=max_post):
            #    print(f"  {ch} data matches", file=self.output)
            #else:
            #    if compare_data(source_data, target_data, self.output, max_pre=max_pre, max_post=max_post, skew=1):
            #        print(f"  {ch} data matches (one-second skew)", file=self.output)
            #    else:
            #        print(f"  {ch} data does not match", file=self.output)
            #        passed = False
            #        print(f"Source: {source_json}", file=self.output)
            #        print(f"Target: {target_json}", file=self.output)
        print(f'  {"PASSED" if passed else "FAILED"} check_sync', file=self.output)
        return passed
