import argparse
import json
import random
import requests
import sys
import posixpath
import urllib.parse

#######################################################################################################################
### Utilities

def some(s):
    return random.choice(sorted(s))

def pretty_json(tree, depth=0, max_depth=None):
    if max_depth is not None  and  depth>=max_depth:
        return
    indent = "  "*depth
    for k,v in tree.items():
        if isinstance(v, int) or isinstance(v, str):
            print(f"{indent}{k}: {v}")
        elif isinstance(v, dict):
            print(f"{indent}{k}: ->")
            pretty_json(v, depth+1)
        else:
            print(f"{indent}{k}: {type(v)}")

#######################################################################################################################
### Data-model and processing

class Param(object):
    def __init__(self, name, location, kind):
        self.location = location
        self.kind     = kind
        self.name     = name
        self.values   = set()

    def dump(self):
        print(f"{self.name} in {self.location} is {self.kind} : {{{self.values}}}")

def does_response_fit_schema(schema, resp):
    #print("Schema:")
    #pretty_json(schema,max_depth=1)
    #print("Response:")
    #pretty_json(resp,max_depth=1)
    if "type" in schema  and  schema["type"]=="object":
        if isinstance(resp,dict)  and  "properties" in schema  and  isinstance(schema["properties"],dict):
            print(f"Validate properties against dictionary")
            for k,v in schema["properties"].items():
                #print(f"Validate {k} received with {v}")
                if not k in resp:
                    print(f"Missing {k} in response")
                    return
                does_response_fit_schema(v, resp[k])
            #pretty_json(schema,max_depth=1)
            #pretty_json(resp,max_depth=1)
        else:
            print(f"Can't understand schema")
            pretty_json(schema,max_depth=1)
    elif "type" in schema  and  schema["type"]=="string":
        if isinstance(resp, str):
            print(f"{repr(resp)} matches {repr(schema)}")
            return
        print(f"FAIL: {repr(resp)} does not match schema {repr(schema)}")
    elif "type" in schema  and  schema["type"]=="boolean":
        if isinstance(resp, bool):
            print(f"{repr(resp)} matches {repr(schema)}")
            return
        print(f"Bool check against {repr(resp)}")
    else:
        print(f"What to do with the schema?")
        pretty_json(schema,max_depth=1)



class GetPath(object):
    def __init__(self, url, spec):
        self.url    = url
        self.req_params = {}
        self.opt_params = {}
        self.success    = None
        self.failures   = {}
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
                if 'enum' in p:
                    for v in p['enum']:
                        target[name].values.add(v)
                if req and len(target[name].values)==0:
                    print(f"FAIL: No default values in swagger for required parameter {name} in {self.url}")
            for code,schema in spec['responses'].items():
                if code=="200" and 'schema' in schema:
                    self.success = schema['schema']
                elif code=="200":
                    print(f"200 reponse with no schema in {self.url}")
                else:
                    self.failures[code] = schema

    def generate_success(self, host):
        args = "&".join([f"{p.name}={some(p.values)}" for p in self.req_params.values() ])
        base_url = urllib.parse.urljoin(host,self.url)
        test_url = f"{base_url}?{args}"
        print(f"TEST: {test_url}")
        #for p in self.req_params.values():
        #    p.dump()
        #for p in self.opt_params.values():
        #    p.dump()
        return requests.get(url=test_url)

    def generate_failure(self, host):
        args = "&".join([f"{p.name}={some(p.values)}" for p in self.req_params.values() ])
        base_url = urllib.parse.urljoin(host,self.url)
        test_url = f"{base_url}?{args}"
        print(f"TEST: {test_url}")
        #for p in self.req_params.values():
        #    p.dump()
        #for p in self.opt_params.values():
        #    p.dump()
        return requests.get(url=test_url)

    def validate(self, resp, expect_success):
        try:
            resp_json = json.loads(resp.text)
        except json.decoder.JSONDecodeError as e:
            print("Non-json response - how to validate?")
            return
        if resp.status_code==200:
            if self.success is not None:
                does_response_fit_schema(self.success, resp_json)
            else:
                print("Missing schema?")
            #print(json.loads(resp.text))



def get_the_spec(url):
    if url[:7] == "file://":
        with open(url[7:]) as f:
            return f.read()
    return requests.get(url=url).text

# Swagger paths look absolute but they are relative to the base.
def not_absolute(path):
    return path[1:] if path[0] == '/' else path

def find_ref(spec, path) :
    if len(path)>0 and path[0] == '#':
        return find_ref(spec,path[1:])
    if len(path)==1:
        return spec[path[0]]
    return find_ref(spec[path[0]], path[1:])

def resolve_refs(spec, spec_root=None):
    '''Find all "$ref" keys in the swagger spec and inline their target schemas.'''
    if spec_root is None:
        spec_root = spec
    newspec = {}
    for k,v in spec.items():
        if k=="$ref":
            path = v.split('/')
            target = find_ref(spec_root, path)
            # Unfold one level of the tree and erase the $ref if possible.
            if isinstance(target,dict):
                for kk,vv in target.items():
                    newspec[kk] = vv
            else:
                newspec[k] = target
        elif isinstance(v,dict):
            newspec[k] = resolve_refs(v, spec_root)
        else:
            newspec[k] = v
    return newspec

#######################################################################################################################
# Initialization

random.seed(7)      # Default is reproducible sequences

parser = argparse.ArgumentParser()
parser.add_argument('--url', type=str,
                    default='https://raw.githubusercontent.com/netdata/netdata/master/web/api/netdata-swagger.json',
                    help='The URL of the API definition in swagger. The default will pull the lastest version ' +
                         'from the main branch.')
parser.add_argument('--host', type=str,
                    help='The URL of the target host to fuzz. The default will read the host from the swagger ' +
                         'definition.')
parser.add_argument('--reseed', action='store_true',
                    help="Pick a random seed for the PRNG. The default uses a constant seed for reproducibility.")

args = parser.parse_args()
if args.reseed:
    random.seed()
spec = json.loads( get_the_spec(args.url) )
inlined_spec = resolve_refs(spec)

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
for name,p in inlined_spec['paths'].items():
    if 'get' in p:
        name = not_absolute(name)
        paths.append(GetPath(posixpath.join(inlined_spec['basePath'],name), p['get']))
    elif 'put' in p:
        print(f"FAIL: Generation of PUT methods (for {name} is unimplemented")

for s in inlined_spec['schemes']:
    for p in paths:
        resp = p.generate_success(s+"://"+host)
        p.validate(resp, True)
        #resp = p.generate_failure(s+"://"+host)
        #p.validate(resp, False)

