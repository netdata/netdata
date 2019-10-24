import argparse
import json
import requests
import sys
import posixpath
import urllib.parse

def some(s):
    return sorted(s)[0]

class Param(object):
    def __init__(self, name, location, kind):
        self.location = location
        self.kind     = kind
        self.name     = name
        self.values   = set()

    def dump(self):
        print(f"{self.name} in {self.location} is {self.kind} : {{{self.values}}}")

class GetPath(object):
    def __init__(self, url, spec):
        self.url    = url
        self.req_params = {}
        self.opt_params = {}
        if 'parameters' in spec.keys():
            for p in spec['parameters']:
                name = p['name']
                req  = p.get('required',False)
                target = self.req_params if req else self.opt_params
                target[name] = Param(name, p['in'], p['type'])
                if 'default' in p:
                    defs = p['default']
                    if isinstance(defs,list):
                        for d in defs:
                            target[name].values.add(d)
                    else:
                        target[name].values.add(defs)
                if req and len(target[name].values)==0:
                    print(f"FAIL: No default values in swagger for required parameter {name} in {self.url}")

    def generate(self, host):
        args = "&".join([f"{p.name}={some(p.values)}" for p in self.req_params.values() ])
        base_url = urllib.parse.urljoin(host,self.url)
        test_url = f"{base_url}?{args}"
        print(f"TEST: {test_url}")


def get_the_spec(url):
    if url[:7] == "file://":
        with open(url[7:]) as f:
            return f.read()
    return requests.get(url=url).text


parser = argparse.ArgumentParser()
parser.add_argument('--url', type=str,
                    default='https://raw.githubusercontent.com/netdata/netdata/master/web/api/netdata-swagger.json',
                    help='The URL of the API definition in swagger. The default will pull the lastest version ' +
                         'from the main branch.')
parser.add_argument('--host', type=str,
                    help='The URL of the target host to fuzz. The default will read the host from the swagger ' +
                         'defintion.')

args = parser.parse_args()
spec = json.loads( get_the_spec(args.url) )

if spec['swagger'] != '2.0':
    print("FAIL: Unexpected swagger version")
    sys.exit(-1)
print(f"INFO: Fuzzing {spec['info']['title']} / {spec['info']['version']}")
host = spec['host']
if args.host is not None:
    host = args.host
    print(host)
print(f"INFO: Target host is {host}")
paths = []
for name,p in spec['paths'].items():
    if 'get' in p:
        paths.append(GetPath(posixpath.join(spec['basePath'],name), p['get']))
    elif 'put' in p:
        print(f"FAIL: Generation of PUT methods (for {name} is unimplemented")

for s in spec['schemes']:
    for p in paths:
        p.generate(s+"://"+host)
