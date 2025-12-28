import crypto from 'node:crypto';

export const sha256Hex = (input: string): string =>
  crypto.createHash('sha256').update(input).digest('hex');
