import functools, math, operator, json, requests, sys

chart = sys.argv[1]
d_prefix = sys.argv[2]
r_prefix = sys.argv[3]

d_url = f"http://{d_prefix}/api/v1/data?chart={chart}"
r_url = f"http://{r_prefix}/api/v1/data?chart={chart}"

# Fetch datasets, check their shape matches
print(f"Direct: {d_url}")
direct = requests.get(d_url).json()
print(f"Remote: {r_url}")
remote = requests.get(r_url).json()

direct_labels = direct["labels"]
remote_labels = remote["labels"]
if direct_labels != remote_labels:
    print( f"Columns differ: {direct_labels} vs {remote_labels}" )
    sys.exit(1)
direct_data = direct["data"]
remote_data = remote["data"]

# Do a range estimation across the charts
all_ds = functools.reduce( operator.add, [ d[1:] for d in direct_data ], [])
all_rs = functools.reduce( operator.add, [ r[1:] for r in remote_data ], [])

max_d, min_d = max(all_ds), min(all_ds)
max_r, min_r = max(all_rs), min(all_rs)

dyn_range = max_d - min_d

print(f"Range estimates: Direct={min_r}-{max_r} Remote={min_d}-{max_d}")

while (len(direct_data)>0 and len(remote_data)>0):
    d = direct_data.pop()
    r = remote_data.pop()
    if d[0] > r[0]:
        print(f"{r[0]} is remote only")
        r = remote_data.pop()
    elif d[0] < r[0]:
        print(f"{d[0]} is direct only")
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
