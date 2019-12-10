import argparse
import json
import logging
import posixpath
import random
import re
import requests
import string
import sys
import urllib.parse

#######################################################################################################################
# Utilities


def some(s):
    return random.choice(sorted(s))


def not_some(s):
    test_set = random.choice([string.ascii_uppercase + string.ascii_lowercase,
                              string.digits,
                              string.digits + ".E-",
                              '0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJK'
                              'LMNOPQRSTUVWXYZ!"#$%\'()*+,-./:;<=>?@[\\]^_`{|}~ '])
    test_len = random.choice([1, 2, 3, 37, 61, 121])
    while True:
        x = ''.join([random.choice(test_set) for _ in range(test_len)])
        if x not in s:
            return x


def build_url(host_maybe_scheme, base_path):
    try:
        if '//' not in host_maybe_scheme:
            host_maybe_scheme = '//' + host_maybe_scheme
        url_tuple = urllib.parse.urlparse(host_maybe_scheme)
        if base_path[0] == '/':
            base_path = base_path[1:]
        return url_tuple.netloc, posixpath.join(url_tuple.path, base_path)
    except Exception as e:
        L.error(f"Critical failure decoding arguments -> {e}")
        sys.exit(-1)


#######################################################################################################################
# Data-model and processing


class Param(object):
    def __init__(self, name, location, kind):
        self.location = location
        self.kind = kind
        self.name = name
        self.values = set()

    def dump(self):
        print(f"{self.name} in {self.location} is {self.kind} : {{{self.values}}}")


def does_response_fit_schema(schema_path, schema, resp):
    '''The schema_path argument tells us where we are (globally) in the schema. The schema argument is the
       sub-tree within the schema json that we are validating against. The resp is the json subtree from the
       target host's response.

       The basic idea is this: swagger defines a model of valid json trees. In this sense it is a formal
       language and we can validate a given server response by checking if the language accepts a particular
       server response. This is basically a parser, but instead of strings we are operating on languages
       of trees.

       This could probably be extended to arbitrary swagger definitions - but the amount of work increases
       rapidly as we attempt to cover the full semantics of languages of trees defined in swagger. Instead
       we have some special cases that describe the parts of the semantics that we've used to describe the
       netdata API.

       If we hit an error (in the schema) that prevents further checks then we return early, otherwise we
       try to collect as many errors as possible.
    '''
    success = True
    if "type" not in schema:
        L.error(f"Cannot progress past {schema_path} -> no type specified in dictionary")
        print(json.dumps(schema, indent=2))
        return False
    if schema["type"] == "object":
        if isinstance(resp, dict) and "properties" in schema and isinstance(schema["properties"], dict):
            L.debug(f"Validate properties against dictionary at {schema_path}")
            for k, v in schema["properties"].items():
                L.debug(f"Validate {k} received with {v}")
                if v.get("required", False) and k not in resp:
                    L.error(f"Missing {k} in response at {schema_path}")
                    print(json.dumps(resp, indent=2))
                    return False
                if k in resp:
                    if not does_response_fit_schema(posixpath.join(schema_path, k), v, resp[k]):
                        success = False
        elif isinstance(resp, dict) and "additionalProperties" in schema \
                and isinstance(schema["additionalProperties"], dict):
            kv_schema = schema["additionalProperties"]
            L.debug(f"Validate additionalProperties against every value in dictionary at {schema_path}")
            if "type" in kv_schema and kv_schema["type"] == "object":
                for k, v in resp.items():
                    if not does_response_fit_schema(posixpath.join(schema_path, k), kv_schema, v):
                        success = False
            else:
                L.error("Don't understand what the additionalProperties means (it has no type?)")
                return False
        else:
            L.error(f"Can't understand schema at {schema_path}")
            print(json.dumps(schema, indent=2))
            return False
    elif schema["type"] == "string":
        if isinstance(resp, str):
            L.debug(f"{repr(resp)} matches {repr(schema)} at {schema_path}")
            return True
        L.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
        return False
    elif schema["type"] == "boolean":
        if isinstance(resp, bool):
            L.debug(f"{repr(resp)} matches {repr(schema)} at {schema_path}")
            return True
        L.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
        return False
    elif schema["type"] == "number":
        if 'nullable' in schema and resp is None:
            L.debug(f"{repr(resp)} matches {repr(schema)} at {schema_path} (because nullable)")
            return True
        if isinstance(resp, int) or isinstance(resp, float):
            L.debug(f"{repr(resp)} matches {repr(schema)} at {schema_path}")
            return True
        L.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
        return False
    elif schema["type"] == "integer":
        if 'nullable' in schema and resp is None:
            L.debug(f"{repr(resp)} matches {repr(schema)} at {schema_path} (because nullable)")
            return True
        if isinstance(resp, int):
            L.debug(f"{repr(resp)} matches {repr(schema)} at {schema_path}")
            return True
        L.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
        return False
    elif schema["type"] == "array":
        if "items" not in schema:
            L.error(f"Schema for array at {schema_path} does not specify items!")
            return False
        item_schema = schema["items"]
        if not isinstance(resp, list):
            L.error(f"Server did not return a list for {schema_path} (typed as array in schema)")
            return False
        for i, item in enumerate(resp):
            if not does_response_fit_schema(posixpath.join(schema_path, str(i)), item_schema, item):
                success = False
    else:
        L.error(f"Invalid swagger type {schema['type']} for {type(resp)} at {schema_path}")
        print(json.dumps(schema, indent=2))
        return False
    return success


class GetPath(object):
    def __init__(self, url, spec):
        self.url = url
        self.req_params = {}
        self.opt_params = {}
        self.success = None
        self.failures = {}
        if 'parameters' in spec.keys():
            for p in spec['parameters']:
                name = p['name']
                req = p.get('required', False)
                target = self.req_params if req else self.opt_params
                target[name] = Param(name, p['in'], p['type'])
                if 'default' in p:
                    defs = p['default']
                    if isinstance(defs, list):
                        for d in defs:
                            target[name].values.add(d)
                    else:
                        target[name].values.add(defs)
                if 'enum' in p:
                    for v in p['enum']:
                        target[name].values.add(v)
                if req and len(target[name].values) == 0:
                    print(f"FAIL: No default values in swagger for required parameter {name} in {self.url}")
        for code, schema in spec['responses'].items():
            if code[0] == "2" and 'schema' in schema:
                self.success = schema['schema']
            elif code[0] == "2":
                L.error(f"2xx response with no schema in {self.url}")
            else:
                self.failures[code] = schema

    def generate_success(self, host):
        url_args = "&".join([f"{p.name}={some(p.values)}" for p in self.req_params.values()])
        base_url = urllib.parse.urljoin(host, self.url)
        test_url = f"{base_url}?{url_args}"
        if url_filter.match(test_url):
            try:
                resp = requests.get(url=test_url, verify=(not args.tls_no_verify))
                self.validate(test_url, resp, True)
            except Exception as e:
                L.error(f"Network failure in test {e}")
        else:
            L.debug(f"url_filter skips {test_url}")

    def generate_failure(self, host):
        all_params = list(self.req_params.values()) + list(self.opt_params.values())
        bad_param = ''.join([random.choice(string.ascii_lowercase) for _ in range(5)])
        while bad_param in all_params:
            bad_param = ''.join([random.choice(string.ascii_lowercase) for _ in range(5)])
        all_params.append(Param(bad_param, "query", "string"))
        url_args = "&".join([f"{p.name}={not_some(p.values)}" for p in all_params])
        base_url = urllib.parse.urljoin(host, self.url)
        test_url = f"{base_url}?{url_args}"
        if url_filter.match(test_url):
            try:
                resp = requests.get(url=test_url, verify=(not args.tls_no_verify))
                self.validate(test_url, resp, False)
            except Exception as e:
                L.error(f"Network failure in test {e}")

    def validate(self, test_url, resp, expect_success):
        try:
            resp_json = json.loads(resp.text)
        except json.decoder.JSONDecodeError as e:
            L.error(f"Non-json response from {test_url}")
            return
        success_code = resp.status_code >= 200 and resp.status_code < 300
        if success_code and expect_success:
            if self.success is not None:
                if does_response_fit_schema(posixpath.join(self.url, str(resp.status_code)), self.success, resp_json):
                    L.info(f"tested {test_url}")
                else:
                    L.error(f"tested {test_url}")
            else:
                L.error(f"Missing schema {test_url}")
        elif not success_code and not expect_success:
            schema = self.failures.get(str(resp.status_code), None)
            if schema is not None:
                if does_response_fit_schema(posixpath.join(self.url, str(resp.status_code)), schema, resp_json):
                    L.info(f"tested {test_url}")
                else:
                    L.error(f"tested {test_url}")
            else:
                L.error("Missing schema for {resp.status_code} from {test_url}")
        else:
            L.error(f"Received incorrect status code {resp.status_code} against {test_url}")


def get_the_spec(url):
    if url[:7] == "file://":
        with open(url[7:]) as f:
            return f.read()
    return requests.get(url=url).text


# Swagger paths look absolute but they are relative to the base.
def not_absolute(path):
    return path[1:] if path[0] == '/' else path


def find_ref(spec, path):
    if len(path) > 0 and path[0] == '#':
        return find_ref(spec, path[1:])
    if len(path) == 1:
        return spec[path[0]]
    return find_ref(spec[path[0]], path[1:])


def resolve_refs(spec, spec_root=None):
    '''Find all "$ref" keys in the swagger spec and inline their target schemas.

       As with all inliners this will break if a definition recursively links to itself, but this should not
       happen in swagger as embedding a structure inside itself would produce a record of infinite size.'''
    if spec_root is None:
        spec_root = spec
    newspec = {}
    for k, v in spec.items():
        if k == "$ref":
            path = v.split('/')
            target = find_ref(spec_root, path)
            # Unfold one level of the tree and erase the $ref if possible.
            if isinstance(target, dict):
                for kk, vv in resolve_refs(target, spec_root).items():
                    newspec[kk] = vv
            else:
                newspec[k] = target
        elif isinstance(v, dict):
            newspec[k] = resolve_refs(v, spec_root)
        else:
            newspec[k] = v
    # This is an artifact of inline the $refs when they are inside a properties key as their children should be
    # pushed up into the parent dictionary. They must be merged (union) rather than replace as we use this to
    # implement polymorphism in the data-model.
    if 'properties' in newspec and isinstance(newspec['properties'], dict) and \
       'properties' in newspec['properties']:
        sub = newspec['properties']['properties']
        del newspec['properties']['properties']
        if 'type' in newspec['properties']:
            del newspec['properties']['type']
        for k, v in sub.items():
            newspec['properties'][k] = v
    return newspec


#######################################################################################################################
# Initialization

random.seed(7)      # Default is reproducible sequences

parser = argparse.ArgumentParser()
parser.add_argument('--url', type=str,
                    default='https://raw.githubusercontent.com/netdata/netdata/master/web/api/netdata-swagger.json',
                    help='The URL of the API definition in swagger. The default will pull the latest version '
                         'from the main branch.')
parser.add_argument('--host', type=str,
                    help='The URL of the target host to fuzz. The default will read the host from the swagger '
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
parser.add_argument('--tls-no-verify', action='store_true',
                    help="Disable TLS certification verification to allow connection to hosts that use"
                         "self-signed certificates")
parser.add_argument('--dump-inlined', action='store_true',
                    help='Dump the inlined swagger spec instead of fuzzing. For "reasons".')

args = parser.parse_args()
if args.reseed:
    random.seed()

spec = json.loads(get_the_spec(args.url))
inlined_spec = resolve_refs(spec)
if args.dump_inlined:
    print(json.dumps(inlined_spec, indent=2))
    sys.exit(-1)

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

host, base_url = build_url(args.host or spec['host'], inlined_spec['basePath'])

L.info(f"Target host is {base_url}")
paths = []
for name, p in inlined_spec['paths'].items():
    if 'get' in p:
        name = not_absolute(name)
        paths.append(GetPath(posixpath.join(base_url, name), p['get']))
    elif 'put' in p:
        L.error(f"Generation of PUT methods (for {name} is unimplemented")

for s in inlined_spec['schemes']:
    for p in paths:
        resp = p.generate_success(s + "://" + host)
        resp = p.generate_failure(s+"://"+host)
