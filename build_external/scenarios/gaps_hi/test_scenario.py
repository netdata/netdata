import functools, math, operator, os, re, requests, sys, time
import AgentTest

me   = os.path.abspath(sys.argv[0])
base = os.path.dirname(me)

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


# Two-node test scenarios. This should be an exhaustive list of the sequences of events that can happen.
# Ascii art is ugly but it shows the target scenario timelines
#   + node restart
#   ^ node network up
#   v node network down
#   - node connected / ready for connection
#   r replication sequence

#  P:  +-^-----         BaselineParentFirst
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

#  P:  +-^--x +^r--     ParentShortRestart (few seconds, socket will reconnect)
#  C:   +^---  ^r--

#  P:  +-^--x   +^r--   ParentLongRestart (multiple minutes, allow timeouts)
#  C:   +^---    ^r--

#  P:  +-^--v  ^r--    ParentShortDisconnect (few seconds, socket will reconnect)
#  C:   +^---  ^r--

#  P:  +-^--v    ^r--  ParentLongDisconnect (multiple minutes, allow timeouts)
#  C:   +^---    ^r--

# Overlapping network disconnection windows

#  P:  +-^--v    ^ -r-  ChildDropOverParentReconnect
#  C:   +^---  v   ^r-

#  P:  +-^--v      ^r-  ChildDropInsideParentReconnect
#  C:   +^---  v ^  r-

#  P:  +-^--   v ^ -r-  ParentDropInsideChildReconnect
#  C:   +^--v      ^r-

#  P:  +-^--   v   ^r-  ParentDropOverChildReconnect
#  C:   +^--v    ^  r-

# TODO: Need long and short variants to check different behaviour on socket reuse....

# Restarts during reconnections

#  P:  +-^--   + ^ -r-  ParentRestartDuringChildReconnect
#  C:   +^--v      ^r-

#  P:  +-^--v     ^r-  ChildRestartDuringParentReconnect
#  C:   +^--   + ^-r-

#  P:  +-^-- v ^  r---  ParentDropDuringChildRestart
#  C:   +^--+++++^r---

#  P:  +-^--+++++^r---  ChildDropDuringParentRestart
#  C:   +^-- v ^  r---

# TODO: max_pre should be 0 for this case when the receiver queries history on first connection
def BaselineParentFirst(state):
    state.start("parent")
    state.wait_up("parent")
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    print("  Measure baseline for 60s...", file=state.output)
    time.sleep(60)
    state.end_checks.append( lambda: state.check_sync("child","parent", max_pre=5) )
    # pylint: disable-msg=W0622
    state.post_checks.append( lambda: state.check_norep() )
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def BaselineChildFirst(state):
    state.start("child")
    state.wait_up("child")
    state.start("parent")
    state.wait_up("parent")
    state.wait_connected("child", "parent")
    print("  Measure baseline for 60s...", file=state.output)
    time.sleep(60)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent",max_pre=10) )
    state.post_checks.append( lambda: state.check_norep() )     # This is not defined in the ask: design choice
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def ChildShortDisconnect(state):
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("child")
    time.sleep(3)
    state.start_net("child")
    time.sleep(5)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def ChildLongDisconnect(state):
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("child")
    time.sleep(30)
    state.start_net("child")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def ChildShortRestart(state):
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.kill("child")
    time.sleep(3)
    state.restart("child")
    state.wait_isparent("parent")
    time.sleep(10)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )
    # TODO: expect difference in charts - test validity check
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def ChildLongRestart(state):
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.kill("child")
    time.sleep(30)
    state.restart("child")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )
    # TODO: expect difference in charts - test validity check
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def ParentShortDisconnect(state):
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("parent")
    time.sleep(3)
    state.start_net("parent")
    state.wait_isparent("parent")
    time.sleep(5)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def ParentLongDisconnect(state):
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("parent")
    time.sleep(30)
    state.start_net("parent")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def ParentShortRestart(state):
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.kill("parent")
    time.sleep(3)
    state.restart("parent")
    state.wait_isparent("parent")
    time.sleep(10)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def ParentLongRestart(state):
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.kill("parent")
    time.sleep(30)
    state.restart("parent")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )
    state.nodes['parent'].parser = state.parser2    # Suppress DNS errors

def ChildDropInsideParentReconnect(state):
#  P:  +-^--v      ^r-  ChildDropInsideParentReconnect
#  C:   +^---  v ^  r-
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("parent")
    time.sleep(5)
    state.stop_net("child")
    time.sleep(3)
    state.start_net("child")
    time.sleep(5)
    state.start_net("parent")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )

def ParentDropInsideChildReconnect(state):
#  P:  +-^--   v ^ -r-  ParentDropInsideChildReconnect
#  C:   +^--v      ^r-
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("child")
    time.sleep(5)
    state.stop_net("parent")
    time.sleep(3)
    state.start_net("parent")
    time.sleep(5)
    state.start_net("child")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )

def ChildDropOverParentReconnect(state):
#  P:  +-^--v    ^ -r-  ChildDropOverParentReconnect
#  C:   +^---  v   ^r-
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("parent")
    time.sleep(5)
    state.stop_net("child")
    time.sleep(3)
    state.start_net("parent")
    time.sleep(5)
    state.start_net("child")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )

def ParentDropOverChildReconnect(state):
#  P:  +-^--   v   ^r-  ParentDropOverChildReconnect
#  C:   +^--v    ^  r-
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("child")
    time.sleep(5)
    state.stop_net("parent")
    time.sleep(3)
    state.start_net("child")
    time.sleep(5)
    state.start_net("parent")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )

def ParentRestartDuringChildReconnect(state):
#  P:  +-^--   + ^ -r-  ParentRestartDuringChildReconnect
#  C:   +^--v      ^r-
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("child")
    time.sleep(5)
    state.kill("parent")
    time.sleep(3)
    state.restart("parent")
    time.sleep(5)
    state.start_net("child")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )

def ChildRestartDuringParentReconnect(state):
#  P:  +-^--v     ^r-  ChildRestartDuringParentReconnect
#  C:   +^--   + ^-r-
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.stop_net("parent")
    time.sleep(5)
    state.kill("child")
    time.sleep(3)
    state.restart("child")
    time.sleep(5)
    state.start_net("parent")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    # Expect gaps (when child is not collecting) but not differences
    state.post_checks.append( lambda: state.check_rep() )

def ParentDropDuringChildRestart(state):
#  P:  +-^-- v ^  r---  ParentDropDuringChildRestart
#  C:   +^--+++++^r---
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.kill("child")
    time.sleep(3)
    state.stop_net("parent")
    time.sleep(5)
    state.start_net("parent")
    time.sleep(3)
    state.restart("child")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    # Expect gaps (when child is not collecting) but not differences
    state.post_checks.append( lambda: state.check_rep() )

def ChildDropDuringParentRestart(state):
#  P:  +-^--+++++^r---  ChildDropDuringParentRestart
#  C:   +^-- v ^  r---
    state.start("parent")
    state.wait_up("parent")
    time.sleep(4)
    state.start("child")
    state.wait_up("child")
    state.wait_connected("child", "parent")
    time.sleep(10)
    state.kill("parent")
    time.sleep(3)
    state.stop_net("child")
    time.sleep(5)
    state.restart("parent")
    time.sleep(3)
    state.start_net("child")
    state.wait_isparent("parent")
    time.sleep(30)
    # pylint: disable-msg=W0622
    state.end_checks.append( lambda: state.check_sync("child","parent") )
    state.post_checks.append( lambda: state.check_rep() )






cases = [
    BaselineChildFirst,
    BaselineParentFirst,
    ChildDropDuringParentRestart,
    ChildDropInsideParentReconnect,
    ChildDropOverParentReconnect,
    ChildLongDisconnect,
    ChildLongRestart,
    ChildRestartDuringParentReconnect,
    ChildShortDisconnect,
    ChildShortRestart,
    ParentDropDuringChildRestart,
    ParentDropInsideChildReconnect,
    ParentDropOverChildReconnect,
    ParentLongDisconnect,
    ParentLongRestart,
    ParentRestartDuringChildReconnect,
    ParentShortDisconnect,
    ParentShortRestart,
]
import argparse
parser = argparse.ArgumentParser()
parser.add_argument('pattern', nargs='?', default=None)
args = parser.parse_args()

configurations = []
for (cdb,pdb) in [("dbengine","dbengine"), ("dbengine","save"), ("save","dbengine"), ("save","save")] :
    state = AgentTest.State(os.path.join(base,"working"), f"{cdb}_{pdb}", f"child={cdb} parent={pdb}")
    c = state.add_node("child")
    c.port = 20000
    c.guid = "22222222-2222-2222-2222-222222222222"
    c.db_mode = cdb
    p = state.add_node("parent")
    p.port = 20001
    p.guid = "11111111-1111-1111-1111-111111111111"
    p.db_mode = pdb
    c.stream_to(p)
    configurations.append(state)

for case in cases:
    if args.pattern is not None:
        print(f"Checking {args.pattern} against {case.__name__}")
        if re.match(args.pattern, case.__name__):
            for state in configurations:
                state.wrap(case)
    else:
        for state in configurations:
            state.wrap(case)
