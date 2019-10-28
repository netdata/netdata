import argparse
import json
import logging
import random
import re
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

def does_response_fit_schema(schema_path, schema, resp):
    '''The schema_path argument tells us where we are (globally) in the schema. The schema argument is the
       sub-tree within the schema json that we are validating against. The resp is the json subtree from the
       target host's response.
    '''
    #print("Schema:")
    #pretty_json(schema,max_depth=1)
    #print("Response:")
    #pretty_json(resp,max_depth=1)
    if "type" in schema  and  schema["type"]=="object":
        if isinstance(resp,dict)  and  "properties" in schema  and  isinstance(schema["properties"],dict):
            L.debug(f"Validate properties against dictionary at {schema_path}")
            for k,v in schema["properties"].items():
                #print(f"Validate {k} received with {v}")
                if not k in resp:
                    L.error(f"Missing {k} in response at {schema_path}")
                    pretty_json(resp)
                    return
                does_response_fit_schema(posixpath.join(schema_path,k), v, resp[k])
            #pretty_json(schema,max_depth=1)
            #pretty_json(resp,max_depth=1)
        else:
            L.error(f"Can't understand schema at {schema_path}")
            pretty_json(schema,max_depth=1)
    elif "type" in schema  and  schema["type"]=="string":
        if isinstance(resp, str):
            L.debug(f"{repr(resp)} matches {repr(schema)} at {schema_path}")
            return
        L.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
    elif "type" in schema  and  schema["type"]=="boolean":
        if isinstance(resp, bool):
            L.debug(f"{repr(resp)} matches {repr(schema)} at {schema_path}")
            return
        L.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
    elif "type" in schema  and  schema["type"] in ("number","integer"):
        if isinstance(resp, int):
            L.debug(f"{repr(resp)} matches {repr(schema)} at {schema_path}")
            return
        L.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
    else:
        L.error(f"What to do with the schema? {type(resp)} at {schema_path}")
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
                    L.error(f"200 reponse with no schema in {self.url}")
                else:
                    self.failures[code] = schema

    def generate_success(self, host):
        args = "&".join([f"{p.name}={some(p.values)}" for p in self.req_params.values() ])
        base_url = urllib.parse.urljoin(host,self.url)
        test_url = f"{base_url}?{args}"
        if url_filter.match(test_url):
            print(f"TEST: {test_url}")
            return requests.get(url=test_url)
        L.debug(f"url_filter skips {test_url}")
        #for p in self.req_params.values():
        #    p.dump()
        #for p in self.opt_params.values():
        #    p.dump()

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
            L.error("Non-json response - how to validate?")
            return
        if resp.status_code==200:
            if self.success is not None:
                does_response_fit_schema(posixpath.join(self.url,"200"), self.success, resp_json)
            else:
                L.error("Missing schema?")
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
            #print(f"CONVERTING {k} {v}")
            path = v.split('/')
            target = find_ref(spec_root, path)
            # Unfold one level of the tree and erase the $ref if possible.
            if isinstance(target,dict):
                for kk,vv in resolve_refs(target,spec_root).items():
                    newspec[kk] = vv
            else:
                newspec[k] = target
        elif isinstance(v,dict):
            newspec[k] = resolve_refs(v, spec_root)
        else:
            #print(f"COPY OVER {k}")
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
parser.add_argument('--passes', action='store_true',
                    help="Log information about tests that pass")
parser.add_argument('--detail', action='store_true',
                    help="Log information about the response/schema comparisons during each test")
parser.add_argument('--filter', type=str,
                    default=".*",
                    help="Supply a regex used to filter the testing URLs generated")

args = parser.parse_args()
if args.reseed:
    random.seed()
spec = json.loads( get_the_spec(args.url) )
inlined_spec = resolve_refs(spec)

#pretty_json(inlined_spec)
#sys.exit(-1)

logging.addLevelName(40, "FAIL")
logging.addLevelName(20, "PASS")
logging.addLevelName(10, "DETAIL")

L = logging.getLogger()
handler = logging.StreamHandler(sys.stdout)
if not args.passes and not args.detail:
    L.setLevel(logging.ERROR)
elif args.passes and not args.detail:
    L.setLevel(logging.INFO)
elif args.detail:
    L.setLevel(logging.DEBUG)
handler.setFormatter(logging.Formatter(fmt="%(levelname)s %(message)s"))
L.addHandler(handler)

url_filter = re.compile(args.filter)

if spec['swagger'] != '2.0':
    L.error(f"Unexpected swagger version")
    sys.exit(-1)
L.info(f"Fuzzing {spec['info']['title']} / {spec['info']['version']}")
host = spec['host']
if args.host is not None:
    host = args.host
L.info(f"Target host is {host}")
paths = []
for name,p in inlined_spec['paths'].items():
    if 'get' in p:
        name = not_absolute(name)
        paths.append(GetPath(posixpath.join(inlined_spec['basePath'],name), p['get']))
    elif 'put' in p:
        L.error(f"Generation of PUT methods (for {name} is unimplemented")

for s in inlined_spec['schemes']:
    for p in paths:
        resp = p.generate_success(s+"://"+host)
        if resp is not None:
            p.validate(resp, True)
        #resp = p.generate_failure(s+"://"+host)
        #p.validate(resp, False)

