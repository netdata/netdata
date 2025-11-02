export const normalizeCallPath = (path: string | undefined): string => {
  if (typeof path !== 'string') return '';
  const segments = path
    .split(':')
    .map((segment) => segment.trim())
    .filter((segment) => segment.length > 0 && segment !== 'tool');
  const normalized: string[] = [];
  segments.forEach((segment) => {
    if (normalized.length > 0 && normalized[normalized.length - 1] === segment) return;
    normalized.push(segment);
  });
  return normalized.join(':');
};

export const appendCallPathSegment = (base: string | undefined, segment: string | undefined): string => {
  const normalizedSegment = typeof segment === 'string' ? segment.trim() : '';
  const normalizedBase = normalizeCallPath(base);
  if (normalizedSegment.length === 0 || normalizedSegment === 'tool') return normalizedBase;
  if (normalizedBase.length === 0) return normalizedSegment;
  const segments = normalizedBase.split(':');
  if (segments[segments.length - 1] === normalizedSegment) return normalizedBase;
  segments.push(normalizedSegment);
  return normalizeCallPath(segments.join(':'));
};
