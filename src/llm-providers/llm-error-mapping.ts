export type LlmErrorKind =
  | 'rate_limit'
  | 'auth_error'
  | 'quota_exceeded'
  | 'model_error'
  | 'timeout'
  | 'network_error';

export const LLM_ERROR_KIND_MEANINGS: Record<LlmErrorKind, { summary: string }> = {
  rate_limit: { summary: 'Too many requests; backoff and retry when possible.' },
  auth_error: { summary: 'Authentication or authorization failure; do not retry.' },
  quota_exceeded: { summary: 'Quota/billing limit reached; do not retry.' },
  model_error: { summary: 'Request rejected by provider/model; retryability varies.' },
  timeout: { summary: 'Request timed out or was aborted; retryable depending on policy.' },
  network_error: { summary: 'Network/transport failure; retryable.' },
};

const MESSAGE_KIND_PATTERNS: Record<LlmErrorKind, string[]> = {
  rate_limit: [
    'rate limit',
    'ratelimit',
    'rate_limit',
    'too many requests',
    'overload',
  ],
  auth_error: [
    'authentication',
    'unauthorized',
    'invalid api key',
    'unauthenticated',
    'access denied',
    'forbidden',
  ],
  quota_exceeded: [
    'quota',
    'billing',
    'insufficient',
    'insufficient_quota',
    'quota exceeded',
    'payment required',
    'credits',
  ],
  model_error: [
    'invalid',
    'model',
    'model not found',
    'unknown model',
    'invalid model',
    'unsupported model',
    'not available',
  ],
  timeout: [
    'timeout',
    'timed out',
    'deadline exceeded',
    'context deadline exceeded',
    'aborted',
    'etimedout',
    'econnaborted',
  ],
  network_error: [
    'network',
    'connection',
    'socket hang up',
    'epipe',
    'eai_again',
    'dns',
    'tls',
    'ssl',
    'certificate',
  ],
};

const STATUS_KIND_MAP = new Map<number, LlmErrorKind>([
  [429, 'rate_limit'],
  [401, 'auth_error'],
  [403, 'auth_error'],
  [402, 'quota_exceeded'],
  [400, 'model_error'],
  [408, 'timeout'],
]);

const NAME_KIND_MAP = new Map<string, LlmErrorKind>([
  ['ratelimiterror', 'rate_limit'],
  ['rate_limit_error', 'rate_limit'],
  ['toomanyrequestserror', 'rate_limit'],
  ['autherror', 'auth_error'],
  ['authenticationerror', 'auth_error'],
  ['unauthorizederror', 'auth_error'],
  ['invalidapikeyerror', 'auth_error'],
  ['billingerror', 'quota_exceeded'],
  ['quotaexceedederror', 'quota_exceeded'],
  ['insufficientquotaerror', 'quota_exceeded'],
  ['badrequesterror', 'model_error'],
  ['invalidrequesterror', 'model_error'],
  ['unsupportedmodelerror', 'model_error'],
  ['modelnotfounderror', 'model_error'],
  ['ai_unsupportedmodelversionerror', 'model_error'],
  ['unsupportedmodelversionerror', 'model_error'],
  ['timeouterror', 'timeout'],
  ['aborterror', 'timeout'],
  ['networkerror', 'network_error'],
  ['connectionerror', 'network_error'],
  ['fetcherror', 'network_error'],
]);

const CODE_KIND_MAP = new Map<string, LlmErrorKind>([
  ['rate_limit', 'rate_limit'],
  ['rate_limit_exceeded', 'rate_limit'],
  ['rate_limited', 'rate_limit'],
  ['too_many_requests', 'rate_limit'],
  ['unauthorized', 'auth_error'],
  ['invalid_api_key', 'auth_error'],
  ['authentication_error', 'auth_error'],
  ['auth_error', 'auth_error'],
  ['quota_exceeded', 'quota_exceeded'],
  ['insufficient_quota', 'quota_exceeded'],
  ['billing_hard_limit_reached', 'quota_exceeded'],
  ['invalid_request', 'model_error'],
  ['invalid_request_error', 'model_error'],
  ['bad_request', 'model_error'],
  ['unsupported_model', 'model_error'],
  ['model_not_found', 'model_error'],
  ['invalid_model', 'model_error'],
  ['timeout', 'timeout'],
  ['request_timeout', 'timeout'],
  ['timed_out', 'timeout'],
  ['etimedout', 'timeout'],
  ['econnaborted', 'timeout'],
  ['abort_error', 'timeout'],
  ['econnreset', 'network_error'],
  ['enotfound', 'network_error'],
  ['enetunreach', 'network_error'],
  ['ehostunreach', 'network_error'],
  ['econnrefused', 'network_error'],
  ['eai_again', 'network_error'],
  ['epipe', 'network_error'],
  ['err_network', 'network_error'],
]);

const NON_RETRYABLE_MODEL_ERROR_NAMES = new Set([
  'ai_unsupportedmodelversionerror',
  'unsupportedmodelversionerror',
  'unsupportedmodelerror',
  'modelnotfounderror',
]);

const NON_RETRYABLE_MODEL_ERROR_CODES = new Set([
  'unsupported_model',
  'model_not_found',
  'invalid_model',
]);

const NON_RETRYABLE_MODEL_ERROR_MESSAGE_PATTERNS = [
  'permanently',
  'unsupported',
  'model not found',
  'unknown model',
  'invalid model',
  'not available',
];

const normalize = (value: string | undefined): string | undefined =>
  typeof value === 'string' ? value.trim().toLowerCase() : undefined;

export const classifyLlmErrorKindFromMessage = (message: string | undefined): LlmErrorKind | undefined => {
  const normalized = normalize(message);
  if (normalized === undefined || normalized.length === 0) return undefined;
  const matches = (kind: LlmErrorKind): boolean =>
    MESSAGE_KIND_PATTERNS[kind].some((pattern) => normalized.includes(pattern));

  if (matches('rate_limit')) return 'rate_limit';
  if (matches('auth_error')) return 'auth_error';
  if (matches('quota_exceeded')) return 'quota_exceeded';
  if (matches('model_error')) return 'model_error';
  if (matches('timeout')) return 'timeout';
  if (matches('network_error')) return 'network_error';
  return undefined;
};

export const classifyLlmErrorKind = (input: { status: number; name: string; code?: string; message?: string }): LlmErrorKind | undefined => {
  const statusKind = STATUS_KIND_MAP.get(input.status);
  const nameKey = normalize(input.name);
  const codeKey = normalize(input.code);
  const nameKind = nameKey !== undefined ? NAME_KIND_MAP.get(nameKey) : undefined;
  const codeKind = codeKey !== undefined ? CODE_KIND_MAP.get(codeKey) : undefined;
  const messageKind = classifyLlmErrorKindFromMessage(input.message);
  const matches = (kind: LlmErrorKind): boolean =>
    statusKind === kind || nameKind === kind || codeKind === kind || messageKind === kind;

  if (matches('rate_limit')) return 'rate_limit';
  if (matches('auth_error')) return 'auth_error';
  if (matches('quota_exceeded')) return 'quota_exceeded';
  if (matches('model_error')) return 'model_error';
  if (matches('timeout')) return 'timeout';
  if (matches('network_error')) return 'network_error';
  if (input.status >= 500 && input.status !== 0) return 'network_error';
  return undefined;
};

export const isRetryableModelError = (input: { name: string; code?: string; message?: string }): boolean => {
  const nameKey = normalize(input.name);
  const codeKey = normalize(input.code);
  const messageKey = normalize(input.message);
  if (nameKey !== undefined && NON_RETRYABLE_MODEL_ERROR_NAMES.has(nameKey)) return false;
  if (codeKey !== undefined && NON_RETRYABLE_MODEL_ERROR_CODES.has(codeKey)) return false;
  if (messageKey !== undefined && NON_RETRYABLE_MODEL_ERROR_MESSAGE_PATTERNS.some((pattern) => messageKey.includes(pattern))) return false;
  return true;
};
