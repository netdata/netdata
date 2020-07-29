import functools, json, math, operator, os, re, requests, sys, time

me   = os.path.abspath(sys.argv[0])
base = os.path.dirname(me)

print(sys.argv)

def sh(x):
    return os.popen(x).read()

def get_data(prefix, chart):
    url = f"http://{prefix}/api/v1/data?chart={chart}"
    try:
        r = requests.get(url)
        return r.json()
    except json.decoder.JSONDecodeError:
        print(f"  Fetch failed {url} -> {r.text}")
        return None
    except requests.exceptions.ConnectionError:
        print(f"  Fetch failed {url} -> connection refused")
        return None

def cmp_data(direct_data, remote_data, remote_name):
    #print(f"Direct: {direct_data}")
    #print(f"Remote: {remote_data}")
    # Work out time ranges
    times_ds = [ d[0] for d in direct_data ]
    times_rs = [ r[0] for r in remote_data ]
    if len(times_ds)==0 or len(times_rs)==0:
        print(f"  Empty dataset returned direct={times_ds} remote={times_rs}")
        return False
    # Do a range estimation across the charts
    try:
        start_d = min(times_ds)
        end_d = max(times_ds)
        start_r = min(times_rs)
        end_r = max(times_rs)
        all_ds = functools.reduce( operator.add, [ d[1:] for d in direct_data ], [])
        all_rs = functools.reduce( operator.add, [ r[1:] for r in remote_data ], [])
        max_d, min_d = max(all_ds), min(all_ds)
        max_r, min_r = max(all_rs), min(all_rs)
    except e:
        print(f"{e} occurred on {all_ds} and {all_rs}")
        return False
    print(f"  Range estimates: Direct={min_d}-{max_d}@{start_d}-{end_d} {remote_name}={min_r}-{max_r}@{start_r}-{end_r}")

    dyn_range = max_d - min_d

    direct_data = direct_data[:]
    remote_data = remote_data[:]
    uniques = 0
    shared = 0
    while (len(direct_data)>0 and len(remote_data)>0):
        d = direct_data.pop()
        r = remote_data.pop()
        if d[0] > r[0]:
            uniques += 1
            if len(remote_data)>0:
                r = remote_data.pop()
            else:
                break
        elif d[0] < r[0]:
            uniques += 1
            if len(direct_data)>0:
                d = direct_data.pop()
            else:
                break
        else:
            ratios = [ math.fabs(d[i]-r[i])/dyn_range for i in range (1,len(d)) ]
            if (max(ratios)>0.01 ):
                print( d[0], d, r)
                return "fail"
            shared += 1
    if uniques < 5 and shared > 3:
        return "success"
    print(f"Below sync thresholds: {uniques} unique and {shared} shared")
    return "retry"


def BaselineMiddleFirst(state):
    state.start("middle")
    state.wait_up("middle")
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    print("  Measure baseline for 60s...")
    time.sleep(60)
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_norep() )

def BaselineChildFirst(state):
    state.start("child")
    state.wait_up("child")
    state.start("middle")
    state.wait_up("middle")
    state.wait_connected("child", "middle")
    print("  Measure baseline for 60s...")
    time.sleep(60)
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_norep() )     # This is not defined in the ask: design choice

def ShortChildDisconnect(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1")
    time.sleep(3)
    sh("docker network connect gaps_hi_default gaps_hi_agent_child_1")
    time.sleep(5)
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

def LongChildDisconnect(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1")
    time.sleep(30)
    sh("docker network connect gaps_hi_default gaps_hi_agent_child_1")
    time.sleep(5)
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

def ShortMiddleDisconnect(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1")
    time.sleep(3)
    sh("docker network connect gaps_hi_default gaps_hi_agent_middle_1")
    state.wait_isparent("middle")
    time.sleep(5)
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

def LongMiddleDisconnect(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1")
    time.sleep(30)
    sh("docker network connect gaps_hi_default gaps_hi_agent_middle_1")
    state.wait_isparent("middle")
    time.sleep(5)
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

def MiddleRestart(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker kill gaps_hi_agent_middle_1")
    time.sleep(3)
    sh("docker start gaps_hi_agent_middle_1")
    time.sleep(10)
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

class Node(object):
    def __init__(self, name, cname):
        self.name = name
        self.container_name = cname
        self.port = None
        self.guid = None
        self.log = None
        self.started = False


class TestState(object):
    def __init__(self, prefix="gaps_hi", network="_default"):
        self.network         = prefix + network
        self.prefix          = prefix
        self.nodes           = {}


    def wipe(self):
        sh(f"docker-compose -f {base}/child-compose.yml -f {base}/middle-compose.yml -f {base}/parent-compose.yml down --remove-orphans")
        self.end_checks = []    # Before the containers are killed
        self.post_checks = []   # After the containers are killed (and the logs are final)
        self.nodes           = {}
        self.nodes['child']  = Node("child", self.prefix + "_agent_child_1")
        self.nodes['middle'] = Node("middle", self.prefix + "_agent_middle_1")
        self.nodes['parent'] = Node("parent", self.prefix + "_agent_parent_1")

    def wrap(self, case):
        print(f"\n---------------> Wipe test state: {case.__name__}\n")
        self.wipe()
        case(self)
        for c in self.end_checks:
            c()
        for n in self.nodes.values():
            if n.started:
                sh(f"docker kill {n.container_name}")
                n.log = f"{case.__name__}-{n.name}.log"
                sh(f"docker logs {n.container_name} >{n.log} 2>&1")
        for c in self.post_checks:
            c()


    def start(self, node):
        sh(f"docker-compose -f {base}/{node}-compose.yml up -d")
        container = json.loads(sh(f"docker inspect {self.nodes[node].container_name}"))
        self.nodes[node].port = container[0]["NetworkSettings"]["Ports"]["19999/tcp"][0]["HostPort"]
        self.nodes[node].started = True

    def wait_up(self, node):
        url = f"http://localhost:{self.nodes[node].port}/api/v1/info"
        print(f"  Waiting for {node} on {url}")
        while True:
            try:
                r = requests.get(url)
                info = requests.get(url).json()
                self.nodes[node].guid = info['uid']
                return
            except requests.ConnectionError:
                print(f"  Waiting for {node}...")
            except json.decoder.JSONDecodeError:
                print(f"  Waiting for {node}...")
            time.sleep(1)

    def wait_connected(self, sender, receiver):
        '''This will detect the *first time* connection of a child to a parent. It looks in the mirrored
           hosts array so on reconnections it will return instantly because the child database already
           exists on the parent.'''
        url = f"http://localhost:{self.nodes[receiver].port}/api/v1/info"
        print(f"  Waiting for {sender} to connect to {receiver}")
        while True:
            try:
                r = requests.get(url)
                info = requests.get(url).json()
            except requests.ConnectionError:
                print(f"  {receiver} not responding...")
                time.sleep(1)
                continue
            if sender in info['mirrored_hosts']:
               print(f"  {sender} in mirrored_hosts on {receiver}")
               return
            print(f"  {receiver} mirrors {info['mirrored_hosts']}...")
            time.sleep(1)

    def wait_isparent(self, node):
        '''This will detect the connection of some child to a parent. It cannot check which one connected
           in scenarios with multiple children of a parent.'''
        url = f"http://localhost:{self.nodes[node].port}/api/v1/info"
        print(f"  Waiting for {node} to become parent")
        while True:
            try:
                r = requests.get(url)
                info = requests.get(url).json()
            except requests.ConnectionError:
                print(f"  {node} not responding...")
                time.sleep(1)
                continue
            if info['host_labels']['_is_parent'] == 'true':
               print(f"  {node} has child connected")
               return
            print(f"  {node} has host labels {info['host_labels']}...")
            time.sleep(1)

    def check_norep(self):
        '''Check that replication did not occur during the test by scanning the logs for debug.'''
        failed = False
        for n in self.nodes.values():
            if n.started and len(sh(f"grep -i replic {n.log}"))>0:
                print(f"  FAILED {n.name} was involved in replication")
                failed = True
        if not failed:
            print(f"  PASSED no replication detected on {n.name}")

    def check_rep(self):
        '''Check that replication did occur during the test by scanning the logs for debug.'''
        for n in self.nodes.values():
            if n.started and len(sh(f"grep -i replic {n.log}"))>0:
                print(f"  PASSED {n.name} was involved in replication")
                return True
        print(f"  FAILED no replication detected on nodes")
        return False


    def check_sync(self, source, target):
        print(f"  check_sync {source} {target}")
        source_json = get_data(f"localhost:{self.nodes[source].port}", "system.cpu")
        if not source_json:
            print(f"  FAILED to check sync looking at http://localhost:{self.nodes[source].port}")
            return
        target_json = get_data(f"localhost:{self.nodes[source].port}", "system.cpu")
        if not target_json:
            print(f"  FAILED to check sync looking at http://localhost:{self.nodes[target].port}")
            return
        if source_json["labels"] != target_json["labels"]:
            print(f"  Mismatch in chart labels: source={source_json['labels']} target={target_json['labels']}")
        source_data = source_json["data"]
        target_data = target_json["data"]

        cmp = cmp_data(source_data, target_data, target)
        if cmp == "fail":
            print("  FAILED in compare")
            print(source_data)
            print(target_data)
        if cmp == "retry":
            print("  RETRY?")
            print(source_data)
            print(target_data)



cases = [
    ShortChildDisconnect,
    LongChildDisconnect,
    ShortMiddleDisconnect,
    LongMiddleDisconnect,
    MiddleRestart,

    BaselineMiddleFirst,
    BaselineChildFirst,
]
import argparse
parser = argparse.ArgumentParser()
parser.add_argument('pattern', nargs='?', default=None)
args = parser.parse_args()

state = TestState()
for c in cases:
    if args.pattern is not None:
        print(f"Checking {args.pattern} against {c.__name__}")
        if re.match(args.pattern, c.__name__):
            state.wrap(c)
    else:
        state.wrap(c)

#class ShortParentDisconnect(object):
#    def body(self):
#        sh("docker network disconnect gaps_hi_default gaps_hi_agent_parent_1")
#        time.sleep(3)
#        sh("docker network connect gaps_hi_default gaps_hi_agent_parent_1")
#
#class LongParentDisconnect(object):
#    def body(self):
#        sh("docker network disconnect gaps_hi_default gaps_hi_agent_parent_1")
#        time.sleep(30)
#        sh("docker network connect gaps_hi_default gaps_hi_agent_parent_1")
#
#class MiddleRestart(object):
#    def body(self):
#        sh("docker kill gaps_hi_agent_middle_1")
#        time.sleep(3)
#        sh("docker start gaps_hi_agent_middle_1")
#        time.sleep(10)
#
#class ParentRestart(object):
#    def body(self):
#        sh("docker kill gaps_hi_agent_parent_1")
#        time.sleep(3)
#        sh("docker start gaps_hi_agent_parent_1")
#        time.sleep(10)
#
#
#test_cases = [
#   BaselineParentFirst(),
#   ShortChildDisconnect(),
#   LongChildDisconnect(),
#   ShortMiddleDisconnect(),
#   LongMiddleDisconnect(),
#   ShortParentDisconnect(),
#   LongParentDisconnect(),
#   MiddleRestart(),
#   ParentRestart()
#]
#
#def cleanup(name):
#    sh("docker kill gaps_hi_agent_child_1 gaps_hi_agent_middle_1 gaps_hi_agent_parent_1");
#    sh(f"docker logs gaps_hi_agent_child_1 2>&1 | grep -v 'collected in the same interpolation' >{name}_child.log")
#    sh(f"docker logs gaps_hi_agent_middle_1 2>&1 | grep -v 'collected in the same interpolation' >{name}_middle.log")
#    sh(f"docker logs gaps_hi_agent_parent_1 2>&1 | grep -v 'collected in the same interpolation' >{name}_parent.log")
#
#for tc in test_cases:
#    name = tc.__class__.__name__
#    print("Wipe test state")
#    sh(f"docker-compose -f {base}/child-compose.yml -f {base}/middle-compose.yml -f {base}/parent-compose.yml down --remove-orphans")
#    sh(f"docker-compose -f {base}/child-compose.yml -f {base}/middle-compose.yml -f {base}/parent-compose.yml up -d")
#    print(f"Pre-test... {name} @{time.time()}")
#    if not check_sync():
#        print(f"ABORTED@{time.time()}")
#        cleanup(name)
#        continue
#    print(f"Test:  {name}@{time.time()}")
#    tc.body()
#    print(f"Finished: {name}@{time.time()}")
#    if not check_sync():
#        print(f"  FAILED")
#    else:
#        print(f"  PASSED")
#    cleanup(name)





