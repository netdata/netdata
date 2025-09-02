#!/usr/bin/env node
import { execSync } from 'node:child_process';

import { build } from 'esbuild';

async function bundleAndPackage() {
  process.stderr.write('Building bundle with esbuild...\n');
  
  await build({
    entryPoints: ['dist/cli.js'],
    bundle: true,
    platform: 'node',
    target: 'node20',
    format: 'cjs',
    outfile: 'dist/bundle.cjs',
    external: [],
    minify: false,
    sourcemap: false,
  });

  process.stderr.write('Creating binary with pkg...\n');
  const target = process.env.PKG_TARGET ?? 'node20-linux-x64';
  execSync(`pkg dist/bundle.cjs -t ${target} -o ../ai-agent --compress GZip`, {
    stdio: 'inherit'
  });
  
  process.stderr.write('Binary created at ../ai-agent\n');
}

bundleAndPackage().catch((error) => {
  process.stderr.write(`Error: ${error instanceof Error ? error.message : String(error)}\n`);
  process.exit(1);
});
