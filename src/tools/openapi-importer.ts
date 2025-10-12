import yaml from 'js-yaml';

import type { RestToolConfig } from '../types.js';

type UnknownRecord = Record<string, unknown>;

function isRecord(val: unknown): val is UnknownRecord {
  return val !== null && typeof val === 'object' && !Array.isArray(val);
}

function asString(val: unknown): string | undefined { return typeof val === 'string' ? val : undefined; }

function normalizePathToName(path: string): string {
  const withoutBraces = path.replace(/[{}]/g, '');
  const cleaned = withoutBraces.replace(/[^a-zA-Z0-9]+/g, '_');
  return cleaned.replace(/^_+|_+$/g, '').toLowerCase();
}

function methodList(): readonly string[] { return ['get','post','put','patch','delete'] as const; }

interface OpenAPIImportOptions {
  baseUrlOverride?: string;
  toolNamePrefix?: string;
  includeMethods?: ('get'|'post'|'put'|'patch'|'delete')[];
  tagFilter?: string[]; // include only operations with any of these tags
}

export function parseOpenAPISpec(input: string | UnknownRecord): UnknownRecord {
  if (typeof input === 'string') {
    // Try YAML first, then JSON
    try { return yaml.load(input) as UnknownRecord; } catch { /* try JSON */ }
    try { return JSON.parse(input) as UnknownRecord; } catch { /* fallthrough */ }
    throw new Error('Failed to parse OpenAPI: unsupported format');
  }
  return input;
}

/**
 * Resolve a $ref reference in the OpenAPI spec
 */
function resolveRef(spec: UnknownRecord, ref: string): unknown {
  // $ref format: "#/components/parameters/PeopleQuery"
  if (!ref.startsWith('#/')) return undefined;
  
  const parts = ref.slice(2).split('/');
  let current: unknown = spec;
  
  // eslint-disable-next-line functional/no-loop-statements
  for (const part of parts) {
    if (!isRecord(current)) return undefined;
    current = current[part];
  }
  
  return current;
}

export function openApiToRestTools(specIn: string | UnknownRecord, opts?: OpenAPIImportOptions): Record<string, RestToolConfig> {
  const spec = parseOpenAPISpec(specIn);
  const out: Record<string, RestToolConfig> = {};

  const serversVal = spec.servers;
  const serversArr: unknown[] = Array.isArray(serversVal) ? serversVal : [];
  let firstServer: UnknownRecord | undefined;
  if (serversArr.length > 0 && isRecord(serversArr[0])) firstServer = serversArr[0];
  const baseFromServer = firstServer !== undefined ? asString(firstServer.url) : undefined;
  const baseUrl = opts?.baseUrlOverride ?? baseFromServer ?? '';

  const pathsVal = spec.paths;
  const paths: UnknownRecord = isRecord(pathsVal) ? pathsVal : {};
  const include = new Set<string>(opts?.includeMethods ?? (methodList() as string[]));
  const tagFilter = Array.isArray(opts?.tagFilter) && opts.tagFilter.length > 0 ? new Set(opts.tagFilter) : undefined;

  Object.entries(paths).forEach(([p, val]) => {
    if (!isRecord(val)) return;
    methodList().forEach((m) => {
      if (!include.has(m)) return;
      const op = val[m];
      if (!isRecord(op)) return;
      const opTagsRaw: unknown[] = Array.isArray(op.tags) ? (op.tags as unknown[]) : [];
      const opTags = opTagsRaw.filter((t: unknown): t is string => typeof t === 'string');
      if (tagFilter !== undefined && !opTags.some((t) => tagFilter.has(t))) return;

      const summary = asString(op.summary) ?? asString(op.operationId) ?? `${m.toUpperCase()} ${p}`;
      const opIdRaw = asString(op.operationId);
      const nameBase = opIdRaw ?? `${m}_${normalizePathToName(p)}`;
      const prefix = opts?.toolNamePrefix ?? '';
      const name = `${prefix}${prefix.length > 0 ? '_' : ''}${nameBase}`;

      // Parameters: collect from path + op level
      const pathParamsA: unknown[] = Array.isArray(val.parameters as unknown[]) ? (val.parameters as unknown[]) : [];
      const opParamsA: unknown[] = Array.isArray(op.parameters as unknown[]) ? (op.parameters as unknown[]) : [];
      
      // Resolve $ref parameters
      const resolvedParams: UnknownRecord[] = [];
      [...pathParamsA, ...opParamsA].forEach((param) => {
        if (isRecord(param)) {
          const refObj = param as Record<string, unknown>;
          const ref = '$ref' in refObj ? asString(refObj.$ref) : undefined;
          if (ref !== undefined) {
            // Resolve the reference
            const resolved = resolveRef(spec, ref);
            if (isRecord(resolved)) {
              resolvedParams.push(resolved);
            }
          } else {
            resolvedParams.push(param);
          }
        }
      });
      
      const allParams = resolvedParams;
      const pathParams = allParams.filter((prm) => prm.in === 'path');
      const queryParams = allParams.filter((prm) => prm.in === 'query');
      const headerParams = allParams.filter((prm) => prm.in === 'header');

      // Build parameters schema
      const parametersSchema: UnknownRecord = { type: 'object', properties: {}, required: [] as string[] };
      const props = parametersSchema.properties as UnknownRecord;
      const req = parametersSchema.required as string[];

      // Path params (required by definition)
      pathParams.forEach((prm) => {
        const nm = asString(prm.name) ?? '';
        if (nm.length === 0) return;
        const schRaw = isRecord(prm.schema) ? prm.schema : undefined;
        props[nm] = schRaw ?? { type: 'string' };
        req.push(nm);
      });

      // Query params (include required only to avoid empty ?x=)
      // Check if any query param has complex schema (object/array)
      let hasComplexQueryParams = false;
      queryParams.forEach((prm) => {
        const nm = asString(prm.name) ?? '';
        if (nm.length === 0) return;
        const required = Boolean(prm.required);
        let schRaw = isRecord(prm.schema) ? prm.schema : undefined;
        
        // Resolve schema $ref if present
        if (schRaw !== undefined && isRecord(schRaw)) {
          const schemaRefObj = schRaw as Record<string, unknown>;
          const schemaRef = '$ref' in schemaRefObj ? asString(schemaRefObj.$ref) : undefined;
          if (schemaRef !== undefined) {
            const resolvedSchema = resolveRef(spec, schemaRef);
            if (isRecord(resolvedSchema)) {
              schRaw = resolvedSchema;
            }
          }
        }
        
        // Check if this is a complex parameter (object or array)
        const schemaType = schRaw?.type;
        if (schemaType === 'object' || schemaType === 'array') {
          hasComplexQueryParams = true;
        }
        
        props[nm] = schRaw ?? { type: 'string' };
        if (required) req.push(nm);
      });

      // Header params (allow templating via ${parameters.*})
      headerParams.forEach((prm) => {
        const nm = asString(prm.name) ?? '';
        if (nm.length === 0) return;
        const schRaw = isRecord(prm.schema) ? prm.schema : undefined;
        props[nm] = schRaw ?? { type: 'string' };
        if (Boolean(prm.required)) req.push(nm);
      });

      // Request body (application/json) => provide a top-level 'body' arg
      let hasBodyTemplate = false;
      let bodyTemplate: unknown;
      const requestBody = isRecord(op.requestBody) ? op.requestBody : undefined;
      const rbContent = requestBody !== undefined && isRecord(requestBody.content) ? requestBody.content : undefined;
      const jsonContentRaw = rbContent !== undefined ? rbContent['application/json'] : undefined;
      const jsonContent = isRecord(jsonContentRaw) ? jsonContentRaw : undefined;
      const bodySchemaRaw = jsonContent !== undefined ? jsonContent.schema : undefined;
      const bodySchema = isRecord(bodySchemaRaw) ? bodySchemaRaw : undefined;
      if (requestBody !== undefined && bodySchema !== undefined) {
        // Provide a raw body arg to maximize compatibility
        props.body = isRecord(bodySchema) ? { type: 'object' } : {};
        if (Boolean(requestBody.required)) req.push('body');
        bodyTemplate = '${parameters.body}';
        hasBodyTemplate = true;
      }

      // URL: replace {param} => ${parameters.param}
      const url = (baseUrl + p).replace(/\{([^}]+)\}/g, (_m, x: string) => '${parameters.' + x + '}');

      const headers: Record<string, string> = {};
      headerParams.forEach((prm) => {
        const nm = asString(prm.name) ?? '';
        if (nm.length === 0) return;
        headers[nm] = '${parameters.' + nm + '}';
      });

      const tool: RestToolConfig = {
        description: summary,
        method: m.toUpperCase() as RestToolConfig['method'],
        url,
        headers: Object.keys(headers).length > 0 ? headers : undefined,
        parametersSchema,
      };
      
      // Mark tools with complex query params for special handling
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
      if (hasComplexQueryParams) {
        (tool as { hasComplexQueryParams?: boolean }).hasComplexQueryParams = true;
        
        // Store the query param names for runtime processing
        const queryParamNames = queryParams
          .map((prm) => asString(prm.name))
          .filter((nm): nm is string => nm !== undefined && nm.length > 0);
        (tool as { queryParamNames?: string[] }).queryParamNames = queryParamNames;
      }
      
      if (hasBodyTemplate) (tool as { bodyTemplate?: unknown }).bodyTemplate = bodyTemplate;
      out[name] = tool;
    });
  });

  return out;
}
