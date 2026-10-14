"""Context owners: the catalogue's ``contexts`` export.

A custom health alert is not in the stock ``alerts`` export, but it attaches to chart contexts, so a consumer can find
the collector it belongs to by context: first an exact documented context, then the longest context prefix. Both come
from the collector and flow records the alert ownership uses (``declared_contexts`` in ``gen_integrations.py``).

Prefixes cover contexts created at runtime. A record's candidates are the longest common prefix of its documented
contexts in each first-segment namespace (never the Agent's ``netdata`` namespace, which every plugin's
self-monitoring charts share) and the prefixes its ``metrics.dynamic_context_prefixes`` declares. A candidate is kept
only when every context another module documents under it also lies under a longer kept prefix of that module, so a
match never takes another collector's contexts; the check repeats until no candidate drops.
"""

from typing import Dict, Iterable, List, Mapping, Sequence, Set

AGENT_NAMESPACE = 'netdata'


def context_subject(record_id: str, plugin: str, module: str, contexts: Iterable[str],
                    declared_prefixes: Iterable[str] = ()) -> dict:
    return {
        'id': record_id,
        'module': f'{plugin}/{module}',
        'contexts': sorted(set(contexts)),
        'declared': sorted(set(declared_prefixes)),
    }


def namespace_prefixes(contexts: Iterable[str]) -> Set[str]:
    """The dot-aligned longest common prefix of the contexts in each first-segment namespace, ending with a dot and
    leaving at least one segment of every context after it."""
    groups: Dict[str, List[List[str]]] = {}
    for context in contexts:
        parts = context.split('.')
        if len(parts) > 1 and parts[0] != AGENT_NAMESPACE:
            groups.setdefault(parts[0], []).append(parts)

    prefixes = set()
    for members in groups.values():
        common = []
        for segments in zip(*(parts[:-1] for parts in members)):
            if len(set(segments)) != 1:
                break
            common.append(segments[0])
        prefixes.add('.'.join(common) + '.')
    return prefixes


def _conflicts(prefix: str, module: str, module_contexts: Mapping[str, Sequence[str]],
               module_prefixes: Mapping[str, Set[str]]) -> List[str]:
    """Contexts other modules document under the prefix without a longer prefix of their own covering them."""
    return sorted(
        f'{other}: {context}'
        for other, contexts in module_contexts.items() if other != module
        for context in contexts
        if context.startswith(prefix)
        and not any(len(own) > len(prefix) and context.startswith(own) for own in module_prefixes.get(other, ()))
    )


def kept_prefixes(subjects: Sequence[dict]) -> Dict[str, Set[str]]:
    """Kept prefixes per subject id."""
    module_contexts: Dict[str, List[str]] = {}
    for subject in subjects:
        module_contexts.setdefault(subject['module'], []).extend(subject['contexts'])
    candidates = {subject['id']: namespace_prefixes(subject['contexts']) | set(subject['declared'])
                  for subject in subjects}

    changed = True
    while changed:
        changed = False
        module_prefixes: Dict[str, Set[str]] = {}
        for subject in subjects:
            module_prefixes.setdefault(subject['module'], set()).update(candidates[subject['id']])
        for subject in subjects:
            for prefix in sorted(candidates[subject['id']]):
                if _conflicts(prefix, subject['module'], module_contexts, module_prefixes):
                    candidates[subject['id']].discard(prefix)
                    changed = True
    return candidates


def declared_prefix_problems(subjects: Sequence[dict]) -> List[str]:
    """Declared prefixes the export drops, with the contexts of other modules that make them ambiguous."""
    kept = kept_prefixes(subjects)
    module_contexts: Dict[str, List[str]] = {}
    module_prefixes: Dict[str, Set[str]] = {}
    for subject in subjects:
        module_contexts.setdefault(subject['module'], []).extend(subject['contexts'])
        module_prefixes.setdefault(subject['module'], set()).update(kept[subject['id']])

    problems = []
    for subject in subjects:
        for prefix in subject['declared']:
            if prefix not in kept[subject['id']]:
                conflicts = _conflicts(prefix, subject['module'], module_contexts, module_prefixes)
                problems.append(f'{subject["id"]}: declared prefix {prefix!r} also covers ' + ', '.join(conflicts[:5]))
    return problems


def context_owners(subjects: Sequence[dict]) -> dict:
    """The ``contexts`` export: ``exact`` maps a documented context and ``prefixes`` a kept prefix to the sorted ids
    of the records that own it."""
    exact: Dict[str, Set[str]] = {}
    prefixes: Dict[str, Set[str]] = {}
    for subject in subjects:
        for context in subject['contexts']:
            exact.setdefault(context, set()).add(subject['id'])
    for subject_id, kept in kept_prefixes(subjects).items():
        for prefix in kept:
            prefixes.setdefault(prefix, set()).add(subject_id)
    return {
        'exact': {context: sorted(ids) for context, ids in sorted(exact.items())},
        'prefixes': {prefix: sorted(ids) for prefix, ids in sorted(prefixes.items())},
    }
