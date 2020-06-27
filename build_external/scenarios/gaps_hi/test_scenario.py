import functools, json, math, operator, os, requests, sys, time

me   = os.path.abspath(sys.argv[0])
base = os.path.dirname(me)

print(sys.argv)

def sh(x):
    return os.popen(x).read()

def get_data(prefix, chart):
    url = f"http://{prefix}/api/v1/data?chart={chart}"
    r = requests.get(url)
    try:
        return requests.get(url).json()
    except json.decoder.JSONDecodeError as e:
        print(f"Fetch failed {url} -> {r.text}")
        return None

def cmp_data(direct_data, remote_data, remote_name):
    # Work out time ranges
    times_ds = [ d[0] for d in direct_data ]
    times_rs = [ r[0] for r in remote_data ]
    print(times_ds,times_rs)
    start_d = min(times_ds)
    end_d = max(times_ds)
    start_r = min(times_rs)
    end_r = max(times_rs)

    # Do a range estimation across the charts
    all_ds = functools.reduce( operator.add, [ d[1:] for d in direct_data ], [])
    all_rs = functools.reduce( operator.add, [ r[1:] for r in remote_data ], [])

    max_d, min_d = max(all_ds), min(all_ds)
    max_r, min_r = max(all_rs), min(all_rs)
    print(f"Range estimates: Direct={min_d}-{max_d}@{start_d}-{end_d} {remote_name}={min_r}-{max_r}@{start_r}-{end_r}")

    dyn_range = max_d - min_d

    direct_data = direct_data[:]
    remote_data = remote_data[:]
    uniques = 0
    while (len(direct_data)>0 and len(remote_data)>0):
        d = direct_data.pop()
        r = remote_data.pop()
        if d[0] > r[0]:
            print(f"{r[0]} is remote only")
            uniques += 1
            r = remote_data.pop()
        elif d[0] < r[0]:
            print(f"{d[0]} is direct only")
            uniques += 1
            d = direct_data.pop()
        else:
            def ratio(x,y):
                if x == 0 and y == 0:
                    return 1
                if y == 0:
                    return x
                return float(x/y)

            ratios = [ math.fabs(d[i]-r[i])/dyn_range for i in range (1,len(d)) ]
            if (max(ratios)>0.01 ):
                print( d[0], d, r)
                return False
    if uniques < 5:
        return True
    return False

sh(f"docker-compose -f {base}/child-compose.yml -f {base}/middle-compose.yml -f {base}/parent-compose.yml down --remove-orphans")
sh(f"docker-compose -f {base}/child-compose.yml -f {base}/middle-compose.yml -f {base}/parent-compose.yml up -d")

while True:
    print("Waiting for sync...")
    time.sleep(3)

    direct_json = get_data("localhost:21002", "system.cpu")
    if not direct_json:
        continue
    middle_json = get_data("localhost:21001/host/child", "system.cpu")
    if not middle_json:
        continue
    parent_json = get_data("localhost:21000/host/child", "system.cpu")
    if not parent_json:
        continue

    if direct_json["labels"] != middle_json["labels"] or middle_json["labels"] != parent_json["labels"]:
        print(f"Mismatch in chart labels: direct={direct_json['labels']} middle={middle_json['labels']} parent={parent_json['labels']}")
        continue

    direct_data = direct_json["data"]
    middle_data = middle_json["data"]
    parent_data = parent_json["data"]

    if not cmp_data(direct_data, middle_data, "middle"):
        print("Child/middle out of sync")
        continue
    if not cmp_data(direct_data, parent_data, "parent"):
        print("Child/parent out of sync")
        continue
    print("Child/middle/parent in sync")
    break





