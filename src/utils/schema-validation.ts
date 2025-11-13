import { compileSchema, draft04, draft06, draft07, draft2019, draft2020 } from 'json-schema-library';

import type { Draft } from 'json-schema-library';

type DraftAlias = 'draft04' | 'draft06' | 'draft07' | 'draft2019' | 'draft2020';

const DRAFT_ALIASES: Record<string, DraftAlias | undefined> = {
  'draft4': 'draft04',
  'draft-4': 'draft04',
  'draft04': 'draft04',
  'draft-04': 'draft04',
  'draft6': 'draft06',
  'draft-6': 'draft06',
  'draft06': 'draft06',
  'draft-06': 'draft06',
  'draft7': 'draft07',
  'draft-7': 'draft07',
  'draft07': 'draft07',
  'draft-07': 'draft07',
  'draft2019': 'draft2019',
  'draft2019-09': 'draft2019',
  'draft-2019': 'draft2019',
  'draft-2019-09': 'draft2019',
  '2019-09': 'draft2019',
  'draft2020': 'draft2020',
  'draft2020-12': 'draft2020',
  'draft-2020': 'draft2020',
  'draft-2020-12': 'draft2020',
  '2020-12': 'draft2020',
};

const DRAFT_DISPLAY: Record<DraftAlias, string> = {
  draft04: 'draft-04',
  draft06: 'draft-06',
  draft07: 'draft-07',
  draft2019: 'draft-2019-09',
  draft2020: 'draft-2020-12',
};

const DRAFT_MAP: Record<DraftAlias, Draft> = {
  draft04,
  draft06,
  draft07,
  draft2019,
  draft2020,
};

export type SchemaDraftTarget = DraftAlias;

export interface SchemaValidationIssue {
  keyword?: string;
  instancePath: string;
  schemaPath: string;
  message?: string;
}

export interface SchemaValidationReport {
  ok: boolean;
  errors: SchemaValidationIssue[];
}

export const normalizeSchemaDraftTarget = (value: string): SchemaDraftTarget => {
  const normalized = value.trim().toLowerCase();
  const target = DRAFT_ALIASES[normalized];
  if (target === undefined) {
    const choices = Object.values(DRAFT_DISPLAY).join(', ');
    throw new Error(`schema validation requires one of: ${choices}`);
  }
  return target;
};

export const schemaDraftDisplayName = (target: SchemaDraftTarget): string => DRAFT_DISPLAY[target];

export const validateSchemaAgainstDraft = (schema: unknown, target: SchemaDraftTarget): SchemaValidationReport => {
  if (!isPlainObject(schema)) {
    return {
      ok: false,
      errors: [{ keyword: 'type', instancePath: '', schemaPath: '', message: 'tool schema must be an object' }],
    };
  }

  const selectedDraft = DRAFT_MAP[target];
  try {
    compileSchema(schema, { drafts: [selectedDraft] });
    const keywordViolations = findKeywordViolations(schema, target);
    if (keywordViolations.length > 0) {
      return { ok: false, errors: keywordViolations };
    }
    return { ok: true, errors: [] };
  } catch (error: unknown) {
    const details = isPlainObject(error) ? error : undefined;
    const message = typeof details?.message === 'string'
      ? details.message
      : error instanceof Error
        ? error.message
        : String(error);
    const issue: SchemaValidationIssue = {
      keyword: typeof details?.keyword === 'string' ? details.keyword : undefined,
      instancePath: typeof details?.path === 'string' ? details.path : '',
      schemaPath: typeof details?.schemaPath === 'string' ? details.schemaPath : '',
      message,
    };
    return { ok: false, errors: [issue] };
  }
};

const isPlainObject = (value: unknown): value is Record<string, unknown> => {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
};

const DRAFT_ORDER: Record<SchemaDraftTarget, number> = {
  draft04: 0,
  draft06: 1,
  draft07: 2,
  draft2019: 3,
  draft2020: 4,
};

const KEYWORD_INTRODUCTION: Record<string, SchemaDraftTarget | undefined> = {
  propertyNames: 'draft06',
  contains: 'draft06',
  const: 'draft06',
  examples: 'draft06',
  if: 'draft07',
  then: 'draft07',
  else: 'draft07',
  $defs: 'draft2019',
  dependentSchemas: 'draft2019',
  dependentRequired: 'draft2019',
  unevaluatedProperties: 'draft2019',
  unevaluatedItems: 'draft2019',
  minContains: 'draft2019',
  maxContains: 'draft2019',
  prefixItems: 'draft2020',
};

const findKeywordViolations = (schema: Record<string, unknown>, target: SchemaDraftTarget): SchemaValidationIssue[] => {
  const violations: SchemaValidationIssue[] = [];
  const stack: { node: Record<string, unknown>; path: string }[] = [{ node: schema, path: '' }];
  const seen = new Set<Record<string, unknown>>();

  // eslint-disable-next-line functional/no-loop-statements -- iterative traversal avoids recursion depth issues
  while (stack.length > 0) {
    const current = stack.pop();
    if (current === undefined) {
      continue;
    }
    const { node, path } = current;
    if (seen.has(node)) {
      continue;
    }
    seen.add(node);

    Object.entries(node).forEach(([key, value]) => {
      const keywordPath = appendPath(path, key);
      const introducedIn = KEYWORD_INTRODUCTION[key];
      if (introducedIn !== undefined && DRAFT_ORDER[target] < DRAFT_ORDER[introducedIn]) {
        const message = `keyword '${key}' requires ${schemaDraftDisplayName(introducedIn)} or newer`;
        violations.push({
          keyword: key,
          instancePath: keywordPath,
          schemaPath: keywordPath,
          message,
        });
      }

      if (isPlainObject(value)) {
        stack.push({ node: value, path: keywordPath });
      } else if (Array.isArray(value)) {
        value.forEach((item, index) => {
          if (isPlainObject(item)) {
            const arrayPath = appendPath(keywordPath, String(index));
            stack.push({ node: item, path: arrayPath });
          }
        });
      }
    });
  }

  return violations;
};

const appendPath = (base: string, segment: string): string => {
  const encoded = segment.replace(/~/g, '~0').replace(/\//g, '~1');
  if (base.length === 0) {
    return `/${encoded}`;
  }
  return `${base}/${encoded}`;
};
