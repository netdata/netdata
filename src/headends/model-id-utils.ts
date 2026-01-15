const pickBase = (sources: readonly (string | undefined)[]): string => {
  const candidate = sources.find((value) => typeof value === 'string' && value.length > 0) ?? 'agent';
  const trimmed = candidate.replace(/\.ai$/iu, '');
  return trimmed.length > 0 ? trimmed : 'agent';
};

export const buildHeadendModelId = (
  sources: readonly (string | undefined)[],
  seen: Set<string>,
  separator: string,
): string => {
  const base = pickBase(sources);
  let candidate = base;
  let counter = 2;
  // eslint-disable-next-line functional/no-loop-statements -- id de-dup requires iteration
  while (seen.has(candidate)) {
    candidate = `${base}${separator}${String(counter)}`;
    counter += 1;
  }
  seen.add(candidate);
  return candidate;
};
