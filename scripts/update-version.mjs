import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, '..');

const computeScript = path.join(repoRoot, 'scripts', 'compute-version.sh');

let version = '0.0.0';
try {
  version = execFileSync('bash', [computeScript], {
    cwd: repoRoot,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'inherit'],
  }).trim();
  if (version.length === 0) version = '0.0.0';
} catch (error) {
  console.error('[ERROR] Failed to compute version, falling back to 0.0.0');
  console.error(error instanceof Error ? error.message : String(error));
}

const targetPath = path.join(repoRoot, 'src', 'version.generated.ts');
const fileContents = `export const VERSION = '${version}';\n`;

try {
  if (!fs.existsSync(targetPath) || fs.readFileSync(targetPath, 'utf8') !== fileContents) {
    fs.writeFileSync(targetPath, fileContents, 'utf8');
  }
} catch (error) {
  console.error('[ERROR] Failed to write version.generated.ts');
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
