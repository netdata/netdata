#!/usr/bin/env python3

import sys
from pathlib import Path

from jsonschema import ValidationError

from gen_integrations import (CATEGORIES_FILE, SINGLE_PATTERN, MULTI_PATTERN, SINGLE_VALIDATOR, MULTI_VALIDATOR,
                              load_yaml, get_category_sets)


def main():
    if len(sys.argv) != 2:
        print(':error:This script takes exactly one argument.')
        return 2

    check_path = Path(sys.argv[1])

    if not check_path.is_file():
        print(f':error file={check_path}:{check_path} does not appear to be a regular file.')
        return 1

    if check_path.match(SINGLE_PATTERN):
        variant = 'single'
        print(f':debug:{check_path} appears to be single-module metadata.')
    elif check_path.match(MULTI_PATTERN):
        variant = 'multi'
        print(f':debug:{check_path} appears to be multi-module metadata.')
    else:
        print(f':error file={check_path}:{check_path} does not match required file name format.')
        return 1

    categories = load_yaml(CATEGORIES_FILE)

    if not categories:
        print(':error:Failed to load categories file.')
        return 2

    _, valid_categories = get_category_sets(categories)

    data = load_yaml(check_path)

    if not data:
        print(f':error file={check_path}:Failed to load data from {check_path}.')
        return 1

    check_modules = []

    if variant == 'single':
        try:
            SINGLE_VALIDATOR.validate(data)
        except ValidationError as e:
            print(f':error file={check_path}:Failed to validate {check_path} against the schema.')
            raise e
        else:
            check_modules.append(data)
    elif variant == 'multi':
        try:
            MULTI_VALIDATOR.validate(data)
        except ValidationError as e:
            print(f':error file={check_path}:Failed to validate {check_path} against the schema.')
            raise e
        else:
            for item in data['modules']:
                item['meta']['plugin_name'] = data['plugin_name']
                check_modules.append(item)
    else:
        print(':error:Internal error encountered.')
        return 2

    failed = False

    for idx, module in enumerate(check_modules):
        invalid_cats = set(module['meta']['monitored_instance']['categories']) - valid_categories

        if invalid_cats:
            print(
                f':error file={check_path}:Invalid categories found in module {idx} in {check_path}: {", ".join(invalid_cats)}.')
            failed = True

    if failed:
        return 1
    else:
        print('{ check_path } is a valid collector metadata file.')
        return 0


if __name__ == '__main__':
    sys.exit(main())
