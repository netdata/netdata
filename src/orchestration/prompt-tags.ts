import crypto from 'node:crypto';

const NONCE_BYTES = 6;

const buildNonce = (): string => crypto.randomBytes(NONCE_BYTES).toString('hex');

const renderAttributes = (attrs?: Record<string, string>): string => {
  if (attrs === undefined) return '';
  const parts = Object.entries(attrs)
    .filter(([, value]) => typeof value === "string" && value.length > 0)
    .map(([key, value]) => `${key}="${value}"`);
  return parts.length > 0 ? ` ${parts.join(' ')}` : '';
};

export const buildTaggedBlock = (
  prefix: string,
  content: string,
  attrs?: Record<string, string>,
): string => {
  const nonce = buildNonce();
  const tagName = `${prefix}__${nonce}`;
  const attrText = renderAttributes(attrs);
  return `<${tagName}${attrText}>\n${content}\n</${tagName}>`;
};

export const buildOriginalUserRequestBlock = (content: string): string =>
  buildTaggedBlock('original_user_request', content);

export const buildAdvisoryBlock = (
  agent: string,
  content: string,
): string => buildTaggedBlock('advisory', content, { agent });

export const buildResponseBlock = (
  agent: string,
  content: string,
): string => buildTaggedBlock('response', content, { agent });

export const joinTaggedBlocks = (blocks: string[]): string =>
  blocks
    .map((block) => block.trim())
    .filter((block) => block.length > 0)
    .join('\n');
