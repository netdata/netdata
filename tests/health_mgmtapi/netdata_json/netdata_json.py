import sys
import json

def orderme(obj):
    if isinstance(obj, list):
        return sorted(orderme(x) for x in obj)
    elif isinstance(obj, dict):
        return sorted((k, orderme(v)) for k, v in obj.items())

    return obj
