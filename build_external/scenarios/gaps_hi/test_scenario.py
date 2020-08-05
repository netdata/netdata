import functools, json, math, operator, os, re, requests, sys, time

me   = os.path.abspath(sys.argv[0])
base = os.path.dirname(me)

import subprocess as subp

def sh(cmdline, output, input=None):
  commonArgs = { 'stdout':subp.PIPE, 'stderr'    :subp.PIPE,
                 'shell' :True} #,      'executable':'/usr/local/bin/bash' }

  print(f"> {cmdline}", file=output)
  if input==None:
    o = subp.Popen(cmdline, **commonArgs) # nosec
  else:
    o = subp.Popen(cmdline, stdin=subp.PIPE, **commonArgs) # nosec
    o.stdin.write(input)

  res = o.communicate() # nosec
  if len(res[1])>0:
      print(f"StdErr {res[1]}", file=output)
  return res[0]

#def sh(x):
#    return os.popen(x).read()

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


def fuzzy_cmp_data(direct_data, remote_data, remote_name):
    print(f"Direct: {direct_data}")
    print(f"Remote: {remote_data}")
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
    except TypeError as e:
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

# Two-node test scenarios. This should be an exhaustive list of the sequences of events that can happen.
# Ascii art is ugly but it shows the target scenario timelines
#   + node restart
#   ^ node network up
#   v node network down
#   - node connected / ready for connection
#   r replication sequence

#  P:  +-^-----         BaselineMiddleFirst
#  C:   +^-----              no replication

#  P:   +^-----         BaselineChildFirst
#  C:  +-^-----              no replication

#  P:  +-^---  ^r--     ChildShortRestart (few seconds, socket will reconnect)
#  C:   +^--x +^r--         will produce gap, verify test validity

#  P:  +-^---    ^r--   ChildLongRestart  (multiple minutes, allow timeouts)
#  C:   +^--x    +^r--       will produce gap, verify test validity

#  P:  +-^---  ^r--     ChildShortDisconnect (few seconds, socket will reconnect)
#  C:   +^--v  ^r--

#  P:  +-^---    ^r--   ChildLongDisconnect (multiple minutes, allow timeouts)
#  C:   +^--v    ^r--

#  P:  +-^--x +^r--     MiddleShortRestart (few seconds, socket will reconnect)
#  C:   +^---  ^r--

#  P:  +-^--x   +^r--   MiddleLongRestart (multiple minutes, allow timeouts)
#  C:   +^---    ^r--

#  P:  +-^--v  ^r--    MiddleShortDisconnect (few seconds, socket will reconnect)
#  C:   +^---  ^r--

#  P:  +-^--v    ^r--  MiddleLongDisconnect (multiple minutes, allow timeouts)
#  C:   +^---    ^r--

# Overlapping network disconnection windows

#  P:  +-^--v    ^ -r-  ChildDropOverMiddleReconnect
#  C:   +^---  v   ^r-

#  P:  +-^--v      ^r-  ChildDropInsideMiddleReconnect
#  C:   +^---  v ^  r-

#  P:  +-^--   v ^ -r-  MiddleDropInsideChildReconnect
#  C:   +^--v      ^r-

#  P:  +-^--   v   ^r-  MiddleDropOverChildReconnect
#  C:   +^--v    ^  r-

# TODO: Need long and short variants to check different behaviour on socket reuse....

# Restarts during reconnections

#  P:  +-^--   + ^ -r-  MiddleRestartDuringChildReconnect
#  C:   +^--v      ^r-

#  P:  +-^--v     ^r-  ChildRestartDuringMiddleReconnect
#  C:   +^--   + ^-r-

#  P:  +-^-- v ^  r---  MiddleDropDuringChildRestart
#  C:   +^--+++++^r---

#  P:  +-^--+++++^r---  ChildDropDuringMiddleRestart
#  C:   +^-- v ^  r---

def BaselineMiddleFirst(state):
    state.start("middle")
    state.wait_up("middle")
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    print("  Measure baseline for 60s...", file=state.output)
    time.sleep(60)
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_norep() )
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def BaselineChildFirst(state):
    state.start("child")
    state.wait_up("child")
    state.start("middle")
    state.wait_up("middle")
    state.wait_connected("child", "middle")
    print("  Measure baseline for 60s...", file=state.output)
    time.sleep(60)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_norep() )     # This is not defined in the ask: design choice
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def ChildShortDisconnect(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(3)
    sh("docker network connect --alias agent_child gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(5)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def ChildLongDisconnect(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(30)
    sh("docker network connect --alias agent_child gaps_hi_default gaps_hi_agent_child_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def ChildShortRestart(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker kill -s INT gaps_hi_agent_child_1", state.output)
    time.sleep(3)
    sh("docker start gaps_hi_agent_child_1", state.output)
    state.wait_isparent("middle")
    time.sleep(10)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )
    # TODO: expect difference in charts - test validity check
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def ChildLongRestart(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker kill -s INT gaps_hi_agent_child_1", state.output)
    time.sleep(30)
    sh("docker start gaps_hi_agent_child_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )
    # TODO: expect difference in charts - test validity check
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def MiddleShortDisconnect(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(3)
    sh("docker network connect --alias agent_middle gaps_hi_default gaps_hi_agent_middle_1", state.output)
    state.wait_isparent("middle")
    time.sleep(5)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def MiddleLongDisconnect(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(30)
    sh("docker network connect --alias agent_middle gaps_hi_default gaps_hi_agent_middle_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def MiddleShortRestart(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker kill gaps_hi_agent_middle_1", state.output)
    time.sleep(3)
    sh("docker start gaps_hi_agent_middle_1", state.output)
    state.wait_isparent("middle")
    time.sleep(10)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def MiddleLongRestart(state):
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker kill -s INT gaps_hi_agent_middle_1", state.output)
    time.sleep(30)
    sh("docker start gaps_hi_agent_middle_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['middle'].parser = state.parser2    # Suppress DNS errors

def ChildDropInsideMiddleReconnect(state):
#  P:  +-^--v      ^r-  ChildDropInsideMiddleReconnect
#  C:   +^---  v ^  r-
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(3)
    sh("docker network connect --alias agent_child gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(5)
    sh("docker network connect --alias agent_middle gaps_hi_default gaps_hi_agent_middle_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

def MiddleDropInsideChildReconnect(state):
#  P:  +-^--   v ^ -r-  MiddleDropInsideChildReconnect
#  C:   +^--v      ^r-
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(3)
    sh("docker network connect --alias agent_middle gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(5)
    sh("docker network connect --alias agent_child gaps_hi_default gaps_hi_agent_child_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

def ChildDropOverMiddleReconnect(state):
#  P:  +-^--v    ^ -r-  ChildDropOverMiddleReconnect
#  C:   +^---  v   ^r-
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(3)
    sh("docker network connect --alias agent_middle gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(5)
    sh("docker network connect --alias agent_child gaps_hi_default gaps_hi_agent_child_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

def MiddleDropOverChildReconnect(state):
#  P:  +-^--   v   ^r-  MiddleDropOverChildReconnect
#  C:   +^--v    ^  r-
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(3)
    sh("docker network connect --alias agent_child gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(5)
    sh("docker network connect --alias agent_middle gaps_hi_default gaps_hi_agent_middle_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

def MiddleRestartDuringChildReconnect(state):
#  P:  +-^--   + ^ -r-  MiddleRestartDuringChildReconnect
#  C:   +^--v      ^r-
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(5)
    sh("docker kill -s INT gaps_hi_agent_middle_1", state.output)
    time.sleep(3)
    sh("docker start gaps_hi_agent_middle_1", state.output)
    time.sleep(5)
    sh("docker network connect --alias agent_child gaps_hi_default gaps_hi_agent_child_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

def ChildRestartDuringMiddleReconnect(state):
#  P:  +-^--v     ^r-  ChildRestartDuringMiddleReconnect
#  C:   +^--   + ^-r-
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(5)
    sh("docker kill -s INT gaps_hi_agent_child_1", state.output)
    time.sleep(3)
    sh("docker start gaps_hi_agent_child_1", state.output)
    time.sleep(5)
    sh("docker network connect --alias agent_middle gaps_hi_default gaps_hi_agent_middle_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    # Expect gaps (when child is not collecting) but not differences
    state.post_checks.append( lambda: state.check_rep() )

def MiddleDropDuringChildRestart(state):
#  P:  +-^-- v ^  r---  MiddleDropDuringChildRestart
#  C:   +^--+++++^r---
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker kill -s INT gaps_hi_agent_child_1", state.output)
    time.sleep(3)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(5)
    sh("docker network connect --alias agent_middle gaps_hi_default gaps_hi_agent_middle_1", state.output)
    time.sleep(3)
    sh("docker start gaps_hi_agent_child_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    # Expect gaps (when child is not collecting) but not differences
    state.post_checks.append( lambda: state.check_rep() )

def ChildDropDuringMiddleRestart(state):
#  P:  +-^--+++++^r---  ChildDropDuringMiddleRestart
#  C:   +^-- v ^  r---
    state.start("middle")
    state.wait_up("middle")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "middle")
    time.sleep(5)
    sh("docker kill -s INT gaps_hi_agent_middle_1", state.output)
    time.sleep(3)
    sh("docker start gaps_hi_agent_middle_1", state.output)
    time.sleep(5)
    sh("docker network disconnect gaps_hi_default gaps_hi_agent_child_1", state.output)
    time.sleep(3)
    sh("docker network connect --alias agent_child gaps_hi_default gaps_hi_agent_child_1", state.output)
    state.wait_isparent("middle")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","middle") )
    state.post_checks.append( lambda: state.check_rep() )

class Node(object):
    def __init__(self, name, cname, parser):
        self.name = name
        self.container_name = cname
        self.port = None
        self.guid = None
        self.log = None
        self.started = False
        self.parser = parser


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




cases = [
    BaselineChildFirst,
    BaselineMiddleFirst,
    ChildDropDuringMiddleRestart,
    ChildDropInsideMiddleReconnect,
    ChildDropOverMiddleReconnect,
    ChildLongDisconnect,
    ChildLongRestart,
    ChildRestartDuringMiddleReconnect,
    ChildShortDisconnect,
    ChildShortRestart,
    MiddleDropDuringChildRestart,
    MiddleDropInsideChildReconnect,
    MiddleDropOverChildReconnect,
    MiddleLongDisconnect,
    MiddleLongRestart,
    MiddleRestartDuringChildReconnect,
    MiddleShortDisconnect,
    MiddleShortRestart,
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





