#!/usr/bin/env python3

import os
import yaml


def parse_yaml(file_path):
    with open(file_path, 'r') as file:
        return yaml.safe_load(file)


def find_metadata_files(root_dir):
    for dirpath, _, filenames in os.walk(root_dir):
        for filename in filenames:
            if filename == 'metadata.yaml':
                yield os.path.join(dirpath, filename)


def print_attributes(data, indent=0):
    if isinstance(data, dict):
        for key, value in data.items():
            if key == 'attributes':
                print('  ' * indent + f'{key}:')
                print_attributes(value, indent + 1)
            elif indent > 0:
                print('  ' * indent + f'{key}:')
                print_attributes(value, indent + 1)
    elif isinstance(data, list):
        for item in data:
            print_attributes(item, indent)
    else:
        print('  ' * indent + str(data))


if __name__ == '__main__':
    root_path = "/home/vk/repos/otel/opentelemetry-collector-contrib/receiver"

    # Step 1

    num_attributes = 0
    num_attributes_with_description = 0
    num_attributes_with_name_override = 0
    num_attributes_with_type = 0

    for metadata_file in find_metadata_files(root_path):
        data = parse_yaml(metadata_file)
        if 'attributes' not in data:
            continue

        for attribute_name, attribute_details in data['attributes'].items():
            num_attributes += 1

            if isinstance(attribute_details, dict) and 'description' in attribute_details:
                num_attributes_with_description += 1

            if isinstance(attribute_details, dict) and 'name_override' in attribute_details:
                num_attributes_with_name_override += 1

            if isinstance(attribute_details, dict) and 'type' in attribute_details:
                num_attributes_with_type += 1

        print(f"Number of attributes in file {metadata_file}: {len(data['attributes'].items())}")

    print(f"Number of attributes with description: {num_attributes_with_description}/{num_attributes}")
    print(f"Number of attributes with name_override: {num_attributes_with_name_override}/{num_attributes}")
    print(f"Number of attributes with type: {num_attributes_with_type}/{num_attributes}")

    # Step 2

    num_metrics = 0
    num_metrics_with_description = 0
    num_metrics_with_enabled = 0
    num_metrics_with_unit = 0
    num_metrics_with_attributes = 0
    num_metrics_with_sum = 0
    num_metrics_with_gauge = 0
    num_metrics_with_extended_documentation = 0
    num_metrics_with_warnings = 0

    for metadata_file in find_metadata_files(root_path):
        data = parse_yaml(metadata_file)
        if 'metrics' not in data:
            continue

        for metric_name, metric_details in data['metrics'].items():
            num_metrics += 1

            if isinstance(metric_details, dict) and 'description' in metric_details:
                num_metrics_with_description += 1

            if isinstance(metric_details, dict) and 'enabled' in metric_details:
                num_metrics_with_enabled += 1

            if isinstance(metric_details, dict) and 'unit' in metric_details:
                num_metrics_with_unit += 1

            if isinstance(metric_details, dict) and 'attributes' in metric_details:
                num_metrics_with_attributes += 1

            if isinstance(metric_details, dict) and 'sum' in metric_details:
                num_metrics_with_sum += 1

            if isinstance(metric_details, dict) and 'gauge' in metric_details:
                num_metrics_with_gauge += 1

            if isinstance(metric_details, dict) and 'extended_documentation' in metric_details:
                num_metrics_with_extended_documentation += 1

            if isinstance(metric_details, dict) and 'warnings' in metric_details:
                num_metrics_with_warnings += 1

            known_keys = set({
                'description', # string
                'enabled', # true or false
                'unit', # string
                'attributes', # sequence
                'sum', # dict
                'gauge', # dict
                'extended_documentation', # string
                'warnings', # string
            })
            diff = set(metric_details) - known_keys
            if len(diff) != 0:
                print(f'Unknown keys in {metadata_file}: {diff}')


        print(f"Number of metrics in file {metadata_file}: {len(data['metrics'].items())}")

    print(f"Number of metrics with description: {num_metrics_with_description}/{num_metrics}")
    print(f"Number of metrics with enabled: {num_metrics_with_enabled}/{num_metrics}")
    print(f"Number of metrics with unit: {num_metrics_with_unit}/{num_metrics}")
    print(f"Number of metrics with attributes: {num_metrics_with_attributes}/{num_metrics}")
    print(f"Number of metrics with sum: {num_metrics_with_sum}/{num_metrics}")
    print(f"Number of metrics with gauge: {num_metrics_with_gauge}/{num_metrics}")
    print(f"Number of metrics with extended_documentation: {num_metrics_with_extended_documentation}/{num_metrics}")
    print(f"Number of metrics with warnings: {num_metrics_with_warnings}/{num_metrics}")

    # Step 3

    num_sums = 0
    num_sums_with_value_type = 0
    num_sums_with_monotonic = 0
    num_sums_with_aggregation_temporality = 0
    num_sums_with_input_type = 0

    for metadata_file in find_metadata_files(root_path):
        data = parse_yaml(metadata_file)
        if 'metrics' not in data:
            continue

        for metric_name, metric_details in data['metrics'].items():
            if isinstance(metric_details, dict) and 'sum' not in metric_details:
                continue

            num_sums +=1

            sum = metric_details['sum']

            if isinstance(sum, dict) and 'value_type' in sum:
                num_sums_with_value_type += 1

            if isinstance(sum, dict) and 'monotonic' in sum:
                num_sums_with_monotonic += 1

            if isinstance(sum, dict) and 'aggregation_temporality' in sum:
                num_sums_with_aggregation_temporality += 1

            if isinstance(sum, dict) and 'input_type' in sum:
                num_sums_with_input_type += 1

            known_keys = set({
                'value_type', # double or int
                'monotonic', # true or false
                'aggregation_temporality', # cumulative
                'input_type', # string
            })
            diff = set(sum) - known_keys
            if len(diff) != 0:
                print(f'Unknown keys in {metadata_file}: {diff}')

    print(f"Number of sums with value_type: {num_sums_with_value_type}/{num_sums}")
    print(f"Number of sums with monotonic: {num_sums_with_value_type}/{num_sums}")
    print(f"Number of sums with aggregation_temporality: {num_sums_with_aggregation_temporality}/{num_sums}")
    print(f"Number of sums with input_type: {num_sums_with_input_type}/{num_sums}")

    # Step 4

    num_gauges = 0
    num_gauges_with_value_type = 0
    num_gauges_with_input_type = 0

    for metadata_file in find_metadata_files(root_path):
        data = parse_yaml(metadata_file)
        if 'metrics' not in data:
            continue

        for metric_name, metric_details in data['metrics'].items():
            if isinstance(metric_details, dict) and 'gauge' not in metric_details:
                continue

            num_gauges +=1

            gauge = metric_details['gauge']

            if isinstance(gauge, dict) and 'value_type' in gauge:
                num_gauges_with_value_type += 1

            if isinstance(gauge, dict) and 'input_type' in gauge:
                num_gauges_with_input_type += 1

            known_keys = set({
                'value_type', # double or int
                'input_type', # string
            })
            diff = set(gauge) - known_keys
            if len(diff) != 0:
                print(f'[4] Unknown keys in {metadata_file}: {diff}')

    print(f"[4] Number of gauges with value_type: {num_gauges_with_value_type}/{num_gauges}")
    print(f"[4] Number of gauges with input_type: {num_gauges_with_input_type}/{num_gauges}")

    # Step 5

    num_resource_attributes = 0
    num_resource_attributes_with_description = 0
    num_resource_attributes_with_enabled = 0
    num_resource_attributes_with_type = 0
    num_resource_attributes_with_enum = 0

    for metadata_file in find_metadata_files(root_path):
        data = parse_yaml(metadata_file)
        if 'resource_attributes' not in data:
            continue

        resource_attributes = data['resource_attributes']
        if resource_attributes is None:
            continue

        for resource_attribute_name, resource_attribute_details in data['resource_attributes'].items():
            if not isinstance(metric_details, dict):
                continue

            num_resource_attributes += 1

            if 'description' in resource_attribute_details:
                num_resource_attributes_with_description += 1

            if 'enabled' in resource_attribute_details:
                num_resource_attributes_with_enabled += 1

            if 'type' in resource_attribute_details:
                num_resource_attributes_with_type += 1

            if 'enum' in resource_attribute_details:
                num_resource_attributes_with_enum += 1
                print(f'{resource_attribute_details["enum"]}')

            known_keys = set({
                'description',  # string literal
                'enabled',  # boolean
                'type',  # string or int
                'enum'  # sequence of strings
            })

            diff = set(resource_attribute_details) - known_keys
            if len(diff) != 0:
                print(f'[5] Unknown keys in {metadata_file}: {diff}')

    print(f"[5] Number of resource attributes with description: {num_resource_attributes_with_description}/{num_resource_attributes}")
    print(f"[5] Number of resource attributes with enabled: {num_resource_attributes_with_enabled}/{num_resource_attributes}")
    print(f"[5] Number of resource attributes with type: {num_resource_attributes_with_type}/{num_resource_attributes}")
    print(f"[5] Number of resource attributes with enum: {num_resource_attributes_with_enum}/{num_resource_attributes}")

    # Step 6

    num_files = 0
    num_resource_attributes = 0
    num_metrics = 0
    num_attributes = 0
    num_scope_name = 0
    num_type = 0

    for metadata_file in find_metadata_files(root_path):
        data = parse_yaml(metadata_file)

        num_files += 1

        if 'resource_attributes' in data:
            num_resource_attributes += 1

        if 'metrics' in data:
            num_metrics += 1

        if 'attributes' in data:
            num_metrics += 1

        if 'scope_name' in data:
            num_scope_name += 1

        if 'type' in data:
            num_type += 1

        known_keys = set({
            'metrics',
            'attributes',
            'resource_attributes',
            'scope_name',
            'type',
            'status',  # ignore
            'tests',  # ignore
        })

        diff = set(data) - known_keys
        if len(diff) != 0:
            print(f'[6] Unknown keys in {metadata_file}: {diff}')

    print(f"[6] Number of resource attributes: {num_resource_attributes}/{num_files}")
    print(f"[6] Number of metrics: {num_metrics}/{num_files}")
    print(f"[6] Number of attributes: {num_attributes}/{num_files}")
    print(f"[6] Number of scope name: {num_scope_name}/{num_files}")
    print(f"[6] Number of type: {num_type}/{num_files}")
