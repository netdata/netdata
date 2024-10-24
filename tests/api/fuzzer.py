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


def get_random_value_from_set(s):
    """Return a random value from a sorted set."""
    return random.choice(sorted(s))


def get_value_not_in_set(s):
    """Generate a random value not in the given set."""
    test_sets = [
        string.ascii_uppercase + string.ascii_lowercase,
        string.digits,
        string.digits + ".E-",
        '0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJK'
        'LMNOPQRSTUVWXYZ!"#$%\'()*+,-./:;<=>?@[\\]^_`{|}~ '
    ]
    test_lengths = [1, 2, 3, 37, 61, 121]  # Avoid magic numbers by making this configurable
    test_set = random.choice(test_sets)
    test_len = random.choice(test_lengths)
    while True:
        candidate_value = ''.join([random.choice(test_set) for _ in range(test_len)])
        if candidate_value not in s:
            return candidate_value


def build_url(host_maybe_scheme, base_path):
    """Build and return a full URL from the given host and base path."""
    try:
        if '//' not in host_maybe_scheme:
            host_maybe_scheme = '//' + host_maybe_scheme
        url_tuple = urllib.parse.urlparse(host_maybe_scheme)
        if base_path.startswith('/'):
            base_path = base_path[1:]
        return url_tuple.netloc, posixpath.join(url_tuple.path, base_path)
    except Exception as e:
        logger.error(f"Critical failure decoding arguments: {e}")
        sys.exit(-1)


#######################################################################################################################
# Data-model and processing


class Param:
    """Represents a parameter with a name, location, type, and values."""

    def __init__(self, name, location, param_type):
        self.location = location
        self.type = param_type
        self.name = name
        self.values = set()

    def dump(self):
        print(f"{self.name} in {self.location} is {self.type} : {{{self.values}}}")


def does_response_fit_schema(schema_path, schema, resp):
    """
    Validate a server response against a Swagger schema.
    """
    success = True

    if "type" not in schema:
        logger.error(f"Cannot progress past {schema_path} -> no type specified in dictionary")
        print(json.dumps(schema, indent=2))
        return False

    if schema["type"] == "object":
        if isinstance(resp, dict) and "properties" in schema and isinstance(schema["properties"], dict):
            for k, v in schema["properties"].items():
                if v.get("required", False) and k not in resp:
                    logger.error(f"Missing {k} in response at {schema_path}")
                    print(json.dumps(resp, indent=2))
                    return False
                if k in resp:
                    if not does_response_fit_schema(posixpath.join(schema_path, k), v, resp[k]):
                        success = False
        elif isinstance(resp, dict) and "additionalProperties" in schema and isinstance(schema["additionalProperties"], dict):
            kv_schema = schema["additionalProperties"]
            for k, v in resp.items():
                if not does_response_fit_schema(posixpath.join(schema_path, k), kv_schema, v):
                    success = False
        else:
            logger.error(f"Can't understand schema at {schema_path}")
            print(json.dumps(schema, indent=2))
            return False
    elif schema["type"] == "string":
        if isinstance(resp, str):
            return True
        logger.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
        return False
    elif schema["type"] == "boolean":
        if isinstance(resp, bool):
            return True
        logger.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
        return False
    elif schema["type"] == "number":
        if 'nullable' in schema and resp is None:
            return True
        if isinstance(resp, (int, float)):
            return True
        logger.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
        return False
    elif schema["type"] == "integer":
        if 'nullable' in schema and resp is None:
            return True
        if isinstance(resp, int):
            return True
        logger.error(f"{repr(resp)} does not match schema {repr(schema)} at {schema_path}")
        return False
    elif schema["type"] == "array":
        if "items" not in schema:
            logger.error(f"Schema for array at {schema_path} does not specify items!")
            return False
        if not isinstance(resp, list):
            logger.error(f"Server did not return a list for {schema_path} (typed as array in schema)")
            return False
        for i, item in enumerate(resp):
            if not does_response_fit_schema(posixpath.join(schema_path, str(i)), schema["items"], item):
                success = False
    else:
        logger.error(f"Invalid swagger type {schema['type']} for {type(resp)} at {schema_path}")
        return False

    return success


class GetPath:
    """Represents an HTTP GET path with required and optional parameters."""

    def __init__(self, url, spec):
        self.url = url
        self.req_params = {}
        self.opt_params = {}
        self.success = None
        self.failures = {}

        if 'parameters' in spec:
            for p in spec['parameters']:
                param = Param(p['name'], p['in'], p['type'])
                target = self.req_params if p.get('required', False) else self.opt_params
                target[p['name']] = param

                if 'default' in p:
                    param.values.update(p['default'] if isinstance(p['default'], list) else [p['default']])

                if 'enum' in p:
                    param.values.update(p['enum'])

                if param.name not in param.values and not param.values:
                    print(f"FAIL: No default values in swagger for required parameter {param.name} in {self.url}")

        for code, schema in spec['responses'].items():
            if code.startswith("2") and 'schema' in schema:
                self.success = schema['schema']
            else:
                self.failures[code] = schema

    def generate_success(self, host):
        url_args = "&".join([f"{p.name}={get_random_value_from_set(p.values)}" for p in self.req_params.values()])
        full_url = urllib.parse.urljoin(host, self.url) + f"?{url_args}"

        if url_filter.match(full_url):
            try:
                response = requests.get(url=full_url, verify=(not args.tls_no_verify))
                self.validate(full_url, response, True)
            except requests.exceptions.RequestException as e:
                logger.error(f"Network failure in test {e}")
        else:
            logger.debug(f"url_filter skips {full_url}")

    def generate_failure(self, host):
        all_params = list(self.req_params.values()) + list(self.opt_params.values())
        bad_param_name = get_value_not_in_set([p.name for p in all_params])
        all_params.append(Param(bad_param_name, "query", "string"))

        url_args = "&".join([f"{p.name}={get_value_not_in_set(p.values)}" for p in all_params])
        full_url = urllib.parse.urljoin(host, self.url) + f"?{url_args}"

        if url_filter.match(full_url):
            try:
                response = requests.get(url=full_url, verify=(not args.tls_no_verify))
                self.validate(full_url, response, False)
            except requests.exceptions.RequestException as e:
                logger.error(f"Network failure in test {e}")

    def validate(self, test_url, resp, expect_success):
        try:
            resp_json = resp.json()
        except json.JSONDecodeError:
            logger.error(f"Non-JSON response from {test_url}")
            return

        if resp.status_code in range(200, 300) and expect_success:
            if self.success:
                if does_response_fit_schema(posixpath.join(self.url, str(resp.status_code)), self.success, resp_json):
                    logger.info(f"tested {test_url}")
                else:
                    logger.error(f"tested {test_url} but response does not match schema")
            else:
                logger.error(f"Missing schema for {test_url}")
        elif resp.status_code not in range(200, 300) and not expect_success:
            failure_schema = self.failures.get(str(resp.status_code))
            if failure_schema and does_response_fit_schema(posixpath.join(self.url, str(resp.status_code)), failure_schema, resp_json):
                logger.info(f"tested {test_url}")
            else:
                logger.error(f"tested {test_url} but response does not match failure schema")
        else:
            logger.error(f"Incorrect status code {resp.status_code} for {test_url}")


def get_the_spec(url):
    """Retrieve Swagger spec from URL or file."""
    if url.startswith("file://"):
        with open(url[7:]) as f:
            return f.read()
    return requests.get(url=url).text


def not_absolute(path):
    """Ensure the path is not absolute."""
    return path[1:] if path.startswith('/') else path


def find_ref(spec, path):
    """Recursively find a reference in the Swagger spec."""
    if path.startswith('#'):
        return
