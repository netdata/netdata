#!/usr/bin/env node
/**
 * Comprehensive test suite for fs-mcp-server.js
 * Tests universal file validation, compact output, symlink loops, and all edge cases
 */

import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import os from 'node:os';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Test configuration
const TEST_DIR = path.join(os.tmpdir(), `fs-mcp-test-${Date.now()}`);
const SERVER_PATH = path.join(__dirname, 'fs-mcp-server.js');
const MCPIGNORE_PATH = path.join(TEST_DIR, '.mcpignore');

// Colors for output
const RED = '\x1b[0;31m';
const GREEN = '\x1b[0;32m';
const YELLOW = '\x1b[1;33m';
const BLUE = '\x1b[0;34m';
const GRAY = '\x1b[0;90m';
const NC = '\x1b[0m'; // No Color

let testsPassed = 0;
let testsFailed = 0;
const testResults = [];
let verbose = false;

// Parse command line arguments
if (process.argv.includes('--verbose') || process.argv.includes('-v')) {
  verbose = true;
  console.log(`${YELLOW}Running in verbose mode${NC}`);
}

async function writeMcpIgnore(content) {
  await fs.promises.writeFile(MCPIGNORE_PATH, content, 'utf8');
}

async function removeMcpIgnore() {
  await fs.promises.rm(MCPIGNORE_PATH, { force: true, recursive: true });
}

// Helper to create test directory structure
async function setupTestEnvironment() {
  console.log(`${BLUE}Setting up test environment in: ${TEST_DIR}${NC}`);

  // Create base test directory
  await fs.promises.mkdir(TEST_DIR, { recursive: true });

  // Create test structure
  await fs.promises.mkdir(path.join(TEST_DIR, 'dir1'));
  await fs.promises.mkdir(path.join(TEST_DIR, 'dir2'));
  await fs.promises.mkdir(path.join(TEST_DIR, 'dir1', 'subdir'));
  await fs.promises.mkdir(path.join(TEST_DIR, 'empty_dir'));

  // Create regular files
  await fs.promises.writeFile(path.join(TEST_DIR, 'file1.txt'), 'Hello World\nLine 2\nLine 3');
  await fs.promises.writeFile(path.join(TEST_DIR, 'file2.md'), '# Header\n\nContent here');
  await fs.promises.writeFile(path.join(TEST_DIR, 'dir1', 'nested.js'), 'const x = 42;\nconsole.log(x);');
  await fs.promises.writeFile(path.join(TEST_DIR, 'dir1', 'subdir', 'deep.txt'), 'Deep file content');

  // Create binary file with invalid UTF-8
  const binaryContent = Buffer.from([0xFF, 0xFE, 0x00, 0x48, 0x65, 0x6C, 0x6C, 0x6F]);
  await fs.promises.writeFile(path.join(TEST_DIR, 'binary.dat'), binaryContent);

  // Create a 100-line file for grep context testing
  const lines = Array.from({ length: 100 }, (_, i) => `Line ${i + 1}: ${i % 10 === 0 ? 'MATCH' : 'content'}`);
  await fs.promises.writeFile(path.join(TEST_DIR, 'hundred.txt'), lines.join('\n'));

  // Create symlinks
  await fs.promises.symlink('file1.txt', path.join(TEST_DIR, 'link_to_file'));
  await fs.promises.symlink('dir1', path.join(TEST_DIR, 'link_to_dir'));
  await fs.promises.symlink('nonexistent', path.join(TEST_DIR, 'broken_link'));
  await fs.promises.symlink('/etc/passwd', path.join(TEST_DIR, 'link_outside'));
  await fs.promises.symlink('../dir1', path.join(TEST_DIR, 'dir2', 'link_relative'));

  // Create symlink loops for testing
  // File loop: loop_file_a -> loop_file_b -> loop_file_c -> loop_file_d -> loop_file_a
  await fs.promises.symlink('loop_file_b', path.join(TEST_DIR, 'loop_file_a'));
  await fs.promises.symlink('loop_file_c', path.join(TEST_DIR, 'loop_file_b'));
  await fs.promises.symlink('loop_file_d', path.join(TEST_DIR, 'loop_file_c'));
  await fs.promises.symlink('loop_file_a', path.join(TEST_DIR, 'loop_file_d'));

  // Directory loop: loop_dir_a/ -> loop_dir_b/ -> loop_dir_c/ -> loop_dir_d/ -> loop_dir_a/
  await fs.promises.mkdir(path.join(TEST_DIR, 'loop_base'));
  await fs.promises.symlink('../loop_dir_b', path.join(TEST_DIR, 'loop_base', 'loop_dir_a'));
  await fs.promises.symlink('../loop_dir_c', path.join(TEST_DIR, 'loop_base', 'loop_dir_b'));
  await fs.promises.symlink('../loop_dir_d', path.join(TEST_DIR, 'loop_base', 'loop_dir_c'));
  await fs.promises.symlink('../loop_base/loop_dir_a', path.join(TEST_DIR, 'loop_base', 'loop_dir_d'));

  // Create project directory for token counting test
  const projectDir = path.join(TEST_DIR, 'project');
  await fs.promises.mkdir(projectDir);
  await fs.promises.mkdir(path.join(projectDir, 'src'));
  await fs.promises.mkdir(path.join(projectDir, 'src', 'components'));
  await fs.promises.writeFile(path.join(projectDir, 'package.json'), '{"name":"test","version":"1.0.0"}');
  await fs.promises.writeFile(path.join(projectDir, 'README.md'), '# Test Project\n\nTest documentation.');
  await fs.promises.writeFile(path.join(projectDir, 'src', 'index.js'), 'console.log("Hello");');
  await fs.promises.writeFile(path.join(projectDir, 'src', 'components', 'Button.js'), 'export const Button = () => {};');

  // Create test directory with symlinks
  await fs.promises.mkdir(path.join(TEST_DIR, 'test_tree'));
  await fs.promises.writeFile(path.join(TEST_DIR, 'test_tree', 'file.txt'), 'content');
  await fs.promises.symlink('file.txt', path.join(TEST_DIR, 'test_tree', 'link_file'));
  await fs.promises.symlink('../dir1', path.join(TEST_DIR, 'test_tree', 'link_dir'));

  // Create test directory with hidden files to verify they are NOT excluded
  await fs.promises.mkdir(path.join(TEST_DIR, 'test_hidden'));
  await fs.promises.writeFile(path.join(TEST_DIR, 'test_hidden', '.hidden_file'), 'hidden content');
  await fs.promises.writeFile(path.join(TEST_DIR, 'test_hidden', 'visible_file'), 'visible content');
  await fs.promises.writeFile(path.join(TEST_DIR, 'test_hidden', '.another_hidden'), 'more hidden');

  // Create default-excluded directories
  await fs.promises.mkdir(path.join(TEST_DIR, '.git'));
  await fs.promises.mkdir(path.join(TEST_DIR, '.git', 'objects'), { recursive: true });
  await fs.promises.writeFile(path.join(TEST_DIR, '.git', 'config'), 'EXCLUDED_DEFAULT');
  await fs.promises.mkdir(path.join(TEST_DIR, 'node_modules'));
  await fs.promises.writeFile(path.join(TEST_DIR, 'node_modules', 'module.js'), 'EXCLUDED_DEFAULT');

  // Create exclusion test structure
  await fs.promises.mkdir(path.join(TEST_DIR, 'excluded_dir'));
  await fs.promises.writeFile(path.join(TEST_DIR, 'excluded_dir', 'secret.txt'), 'EXCLUDED_SECRET');
  await fs.promises.writeFile(path.join(TEST_DIR, 'excluded_file.txt'), 'EXCLUDED_FILE');
  await fs.promises.writeFile(path.join(TEST_DIR, 'allowed_file.txt'), 'EXCLUDED_ALLOWED');
  await fs.promises.symlink('excluded_file.txt', path.join(TEST_DIR, 'link_to_excluded'));

  // Create nested exclusion scope for Tree/Find tests
  await fs.promises.mkdir(path.join(TEST_DIR, 'exclude_scope'));
  await fs.promises.writeFile(path.join(TEST_DIR, 'exclude_scope', 'visible.txt'), 'VISIBLE');
  await fs.promises.writeFile(path.join(TEST_DIR, 'exclude_scope', 'excluded_child.txt'), 'EXCLUDED_CHILD');
  await fs.promises.mkdir(path.join(TEST_DIR, 'exclude_scope', 'excluded_child_dir'));
  await fs.promises.writeFile(path.join(TEST_DIR, 'exclude_scope', 'excluded_child_dir', 'child.txt'), 'EXCLUDED_CHILD_DIR');
}

// MCP client implementation
class MCPTestClient {
  constructor(serverArgs = []) {
    this.server = null;
    this.buffer = '';
    this.messageId = 1;
    this.pendingRequests = new Map();
    this.errorOutput = '';
    this.serverArgs = Array.isArray(serverArgs) ? serverArgs : [];
  }

  async start() {
    return new Promise((resolve, reject) => {
      this.server = spawn('node', [SERVER_PATH, ...this.serverArgs, TEST_DIR], {
        stdio: ['pipe', 'pipe', 'pipe']
      });

      this.server.stdout.on('data', (data) => {
        this.buffer += data.toString();
        this.processBuffer();
      });

      this.server.stderr.on('data', (data) => {
        this.errorOutput += data.toString();
        if (verbose) {
          console.error(`${GRAY}[stderr] ${data.toString()}${NC}`);
        }
      });

      this.server.on('error', reject);

      // Initialize
      this.sendRequest('initialize', {
        protocolVersion: '2024-11-05',
        clientInfo: { name: 'test-client', version: '1.0.0' },
        capabilities: {}
      }).then(resolve).catch(reject);
    });
  }

  async stop() {
    if (this.server) {
      this.server.stdin.end();
      this.server.kill('SIGTERM');
      this.server = null;
    }
  }

  processBuffer() {
    const lines = this.buffer.split('\n');
    this.buffer = lines.pop() || '';

    for (const line of lines) {
      if (line.trim() === '') continue;
      try {
        const msg = JSON.parse(line);
        if (msg.id && this.pendingRequests.has(msg.id)) {
          const { resolve, reject } = this.pendingRequests.get(msg.id);
          this.pendingRequests.delete(msg.id);
          if (msg.error) {
            reject(new Error(msg.error.message));
          } else {
            resolve(msg.result);
          }
        }
      } catch (e) {
        // Ignore parse errors
      }
    }
  }

  sendRequest(method, params) {
    return new Promise((resolve, reject) => {
      const id = this.messageId++;
      this.pendingRequests.set(id, { resolve, reject });
      const message = JSON.stringify({
        jsonrpc: '2.0',
        id,
        method,
        params
      }) + '\n';
      this.server.stdin.write(message);
    });
  }

  async callTool(name, args) {
    const result = await this.sendRequest('tools/call', {
      name,
      arguments: args
    });
    return result?.content?.[0]?.text || result;
  }
}

// Test runner
async function runTest(name, testFn, serverArgs = [], setup, teardown) {
  const testNum = testsPassed + testsFailed + 1;

  if (verbose) {
    console.log(`\n${BLUE}═══ Test ${testNum}: ${name} ═══${NC}`);
  } else {
    process.stdout.write(`${GRAY}[${testNum}/${testResults.length}] ${name}...${NC} `);
  }

  const client = new MCPTestClient(serverArgs);
  try {
    if (typeof setup === 'function') {
      await setup();
    }
    await client.start();
    await testFn(client);

    testsPassed++;
    if (verbose) {
      console.log(`${GREEN}✓ PASSED${NC}`);
    } else {
      console.log(`${GREEN}✓${NC}`);
    }

    return { name, passed: true };
  } catch (error) {
    testsFailed++;
    if (verbose) {
      console.log(`${RED}✗ FAILED: ${error.message}${NC}`);
    } else {
      console.log(`${RED}✗${NC}`);
    }

    return { name, passed: false, error: error.message };
  } finally {
    await client.stop();
    if (typeof teardown === 'function') {
      await teardown();
    }
  }
}

// Test definitions
const tests = [
  // Basic operations
  {
    name: 'ListDir basic',
    fn: async (client) => {
      const result = await client.callTool('ListDir', { dir: '.' });
      if (verbose) {
        console.log(`${GRAY}Tool: ListDir${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: '.' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('dir1/')) throw new Error('Missing dir1');
      if (!result.includes('file1.txt')) throw new Error('Missing file1.txt');
      if (!result.includes('link_to_file -> file1.txt')) throw new Error('Missing symlink info');
    }
  },

  {
    name: 'ListDir with all options',
    fn: async (client) => {
      const result = await client.callTool('ListDir', {
        dir: 'project',
        showSize: true,
        showLastModified: true,
        showCreated: true
      });
      if (verbose) {
        console.log(`${GRAY}Tool: ListDir${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          dir: 'project',
          showSize: true,
          showLastModified: true,
          showCreated: true
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('src/')) throw new Error('Missing src directory');
      if (!result.includes('README.md')) throw new Error('Missing README.md');
      if (!result.includes('33B')) throw new Error('Missing size column');
      if (result.includes('src/  ') && result.includes('33B')) throw new Error('Should not show size for directories');
    }
  },

  {
    name: 'Read regular file',
    fn: async (client) => {
      const result = await client.callTool('Read', { file: 'file1.txt', start: 0, lines: 100, headOrTail: 'head' });
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ file: 'file1.txt', start: 0, lines: 100, headOrTail: 'head' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('Hello World')) throw new Error('Content mismatch');
      if (!result.includes('Line 2')) throw new Error('Missing line 2');
    }
  },

  {
    name: 'Read symlink to valid file',
    fn: async (client) => {
      const result = await client.callTool('Read', { file: 'link_to_file', start: 0, lines: 100, headOrTail: 'head' });
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ file: 'link_to_file', start: 0, lines: 100, headOrTail: 'head' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('Hello World')) throw new Error('Should read through valid symlink');
    }
  },

  {
    name: 'Read broken symlink (should fail)',
    fn: async (client) => {
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ file: 'broken_link', start: 0, lines: 100, headOrTail: 'head' })}${NC}`);
      }
      try {
        await client.callTool('Read', { file: 'broken_link', start: 0, lines: 100, headOrTail: 'head' });
        throw new Error('Should have failed on broken symlink');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        }
        if (!e.message.includes('broken_link') || !e.message.includes('nonexistent')) {
          throw new Error('Error should mention symlink and target');
        }
      }
    }
  },

  {
    name: 'Read symlink outside root (should fail)',
    fn: async (client) => {
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ file: 'link_outside', start: 0, lines: 100, headOrTail: 'head' })}${NC}`);
      }
      try {
        await client.callTool('Read', { file: 'link_outside', start: 0, lines: 100, headOrTail: 'head' });
        throw new Error('Should have failed on symlink outside root');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        }
        if (!e.message.includes('outside') || !e.message.includes('/etc/passwd')) {
          throw new Error('Error should mention outside root and target');
        }
      }
    }
  },

  {
    name: 'Read file symlink loop (should fail)',
    fn: async (client) => {
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ file: 'loop_file_a', start: 0, lines: 100, headOrTail: 'head' })}${NC}`);
      }
      try {
        await client.callTool('Read', { file: 'loop_file_a', start: 0, lines: 100, headOrTail: 'head' });
        throw new Error('Should have failed on symlink loop');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        }
        if (!e.message.includes('loop') || !e.message.includes('loop_file')) {
          throw new Error('Error should mention loop');
        }
      }
    }
  },

  // Server intentionally doesn't have Write, Delete, or Move tools

  {
    name: 'Find with glob',
    fn: async (client) => {
      const result = await client.callTool('Find', { dir: 'project', glob: '*.js' });
      if (verbose) {
        console.log(`${GRAY}Tool: Find${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'project', glob: '*.js' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('src/index.js')) throw new Error('Missing index.js');
      if (!result.includes('src/components/Button.js')) throw new Error('Missing Button.js');
      if (result.includes('project/src/index.js')) throw new Error('Should not include base dir prefix');
    }
  },

  {
    name: 'Tree basic',
    fn: async (client) => {
      const result = await client.callTool('Tree', { dir: 'dir1' });
      if (verbose) {
        console.log(`${GRAY}Tool: Tree${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'dir1' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('nested.js')) throw new Error('Missing nested.js');
      if (!result.includes('subdir/')) throw new Error('Missing subdir');
      if (!result.includes('deep.txt')) throw new Error('Missing deep.txt');
    }
  },

  {
    name: 'Tree root allowed by default',
    fn: async (client) => {
      const result = await client.callTool('Tree', { dir: '.' });
      if (verbose) {
        console.log(`${GRAY}Tool: Tree${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: '.' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('under .')) throw new Error('Tree root summary should mention "."');
    }
  },

  {
    name: 'Tree root blocked with --no-tree-root',
    serverArgs: ['--no-tree-root'],
    fn: async (client) => {
      try {
        await client.callTool('Tree', { dir: '.' });
        throw new Error('Should have failed on root Tree with --no-tree-root');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        }
        if (!String(e.message).includes('--no-tree-root')) {
          throw new Error('Error should mention --no-tree-root');
        }
      }
    }
  },

  {
    name: 'Tree with symlinks shows warnings',
    fn: async (client) => {
      const result = await client.callTool('Tree', { dir: 'test_tree' });
      if (verbose) {
        console.log(`${GRAY}Tool: Tree${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'test_tree' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('WARNING: link_file -> file.txt not followed')) throw new Error('Missing file symlink warning');
      if (!result.includes('WARNING: link_dir -> ../dir1')) throw new Error('Missing dir symlink warning');
    }
  },

  {
    name: 'Tree with directory symlink loop',
    fn: async (client) => {
      const result = await client.callTool('Tree', { dir: 'loop_base' });
      if (verbose) {
        console.log(`${GRAY}Tool: Tree${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'loop_base' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      // Should show warnings for all loop symlinks
      if (!result.includes('WARNING: loop_dir_a')) throw new Error('Missing loop_dir_a warning');
      if (!result.includes('WARNING: loop_dir_b')) throw new Error('Missing loop_dir_b warning');
      if (!result.includes('WARNING: loop_dir_c')) throw new Error('Missing loop_dir_c warning');
      if (!result.includes('WARNING: loop_dir_d')) throw new Error('Missing loop_dir_d warning');

      // Check that it doesn't infinitely recurse
      const warningCount = (result.match(/WARNING:/g) || []).length;
      if (warningCount !== 4) throw new Error(`Expected 4 warnings, got ${warningCount}`);
    }
  },


  {
    name: 'Hidden files are included by default - ListDir',
    fn: async (client) => {
      // Test that hidden files (starting with .) are included by default
      const result = await client.callTool('ListDir', { dir: 'test_hidden' });
      if (verbose) {
        console.log(`${GRAY}Tool: ListDir${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'test_hidden' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      // Verify that hidden files ARE included
      if (!result.includes('.hidden_file')) throw new Error('Hidden files should be included by default');
      if (!result.includes('.another_hidden')) throw new Error('Missing .another_hidden file');
      if (!result.includes('visible_file')) throw new Error('Missing visible file');
    }
  },

  {
    name: 'Hidden files are included by default - Tree',
    fn: async (client) => {
      // Test that Tree also shows hidden files
      const result = await client.callTool('Tree', { dir: 'test_hidden' });
      if (verbose) {
        console.log(`${GRAY}Tool: Tree${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'test_hidden' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('.hidden_file')) throw new Error('Tree should show hidden files');
      if (!result.includes('.another_hidden')) throw new Error('Tree should show all hidden files');
      if (!result.includes('visible_file')) throw new Error('Tree should show visible files');
    }
  },

  {
    name: 'Hidden files are included by default - Find',
    fn: async (client) => {
      // Test that Find also matches hidden files with wildcard
      const result = await client.callTool('Find', { dir: 'test_hidden', glob: '*' });
      if (verbose) {
        console.log(`${GRAY}Tool: Find${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'test_hidden', glob: '*' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      // Find with * should match ALL files including hidden ones
      if (!result.includes('.hidden_file')) throw new Error('Find should match hidden files');
      if (!result.includes('.another_hidden')) throw new Error('Find should match all hidden files');
      if (!result.includes('visible_file')) throw new Error('Find should match visible files');
    }
  },

  {
    name: 'Hidden files are included by default - RGrep',
    fn: async (client) => {
      // Test that RGrep searches hidden files
      const result = await client.callTool('RGrep', { dir: 'test_hidden', regex: 'hidden', caseSensitive: true });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'test_hidden', regex: 'hidden', caseSensitive: true })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      // RGrep should find matches in files containing 'hidden'
      // Both .hidden_file and .another_hidden contain the word 'hidden'
      if (!result.includes('.hidden_file') && !result.includes('.another_hidden')) {
        throw new Error('RGrep should search hidden files');
      }
      // At least one hidden file should be found
      if (!result.includes('.')) throw new Error('RGrep should include hidden files in search');
    }
  },

  {
    name: 'Grep basic',
    fn: async (client) => {
      const result = await client.callTool('Grep', {
        file: 'file1.txt',
        regex: 'Line',
        caseSensitive: true,
        before: 0,
        after: 0
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          file: 'file1.txt',
          regex: 'Line',
          caseSensitive: true,
          before: 0,
          after: 0
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('Line 2')) throw new Error('Missing line 2');
      if (!result.includes('Line 3')) throw new Error('Missing line 3');
    }
  },

  {
    name: 'Grep case insensitive',
    fn: async (client) => {
      const result = await client.callTool('Grep', {
        file: 'file1.txt',
        regex: 'hello',
        caseSensitive: false,
        before: 0,
        after: 0
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          file: 'file1.txt',
          regex: 'hello',
          caseSensitive: false,
          before: 0,
          after: 0
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('Hello World')) throw new Error('Should match case-insensitive');
    }
  },

  {
    name: 'Grep with context (before/after)',
    fn: async (client) => {
      const result = await client.callTool('Grep', {
        file: 'hundred.txt',
        regex: 'Line 50',
        caseSensitive: true,
        before: 2,
        after: 3
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          file: 'hundred.txt',
          regex: 'Line 50',
          caseSensitive: true,
          before: 2,
          after: 3
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      // Should show lines 48-53 (2 before, 3 after)
      if (!result.includes('48')) throw new Error('Missing before context line 48');
      if (!result.includes('49')) throw new Error('Missing before context line 49');
      if (!result.includes('50')) throw new Error('Missing matched line 50');
      if (!result.includes('51')) throw new Error('Missing after context line 51');
      if (!result.includes('52')) throw new Error('Missing after context line 52');
      if (!result.includes('53')) throw new Error('Missing after context line 53');
    }
  },

  {
    name: 'Grep no match',
    fn: async (client) => {
      const result = await client.callTool('Grep', {
        file: 'file1.txt',
        regex: 'nonexistent',
        caseSensitive: true,
        before: 0,
        after: 0
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          file: 'file1.txt',
          regex: 'nonexistent',
          caseSensitive: true,
          before: 0,
          after: 0
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('0 matches found')) throw new Error('Should indicate no matches');
    }
  },

  {
    name: 'Grep through symlink',
    fn: async (client) => {
      const result = await client.callTool('Grep', {
        file: 'link_to_file',
        regex: 'Hello',
        caseSensitive: true,
        before: 0,
        after: 0
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          file: 'link_to_file',
          regex: 'Hello',
          caseSensitive: true,
          before: 0,
          after: 0
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('Hello World')) throw new Error('Should grep through valid symlink');
    }
  },

  {
    name: 'Grep with multiple non-overlapping matches',
    fn: async (client) => {
      // Search for "Line 10", "Line 30", "Line 50" pattern - they're far apart
      const result = await client.callTool('Grep', {
        file: 'hundred.txt',
        regex: 'Line (10|30|50):',
        caseSensitive: true,
        before: 1,
        after: 1
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          file: 'hundred.txt',
          regex: 'Line (10|30|50):',
          caseSensitive: true,
          before: 1,
          after: 1
        })}${NC}`);
        console.log('Output:');
        console.log(result);
        console.log(`${GRAY}Note: Shows 3 separate match blocks with context${NC}`);
      }
      if (!result.includes('3 matches found')) throw new Error('Should find 3 matches');
      // Check that we have 3 separate blocks (separated by ───)
      const separatorCount = (result.match(/───/g) || []).length;
      if (separatorCount !== 2) throw new Error(`Expected 2 separators (3 blocks), got ${separatorCount}`);
    }
  },

  {
    name: 'Grep with multiple overlapping matches',
    fn: async (client) => {
      // Search for lines 50, 52, 54 - with context they will overlap
      const result = await client.callTool('Grep', {
        file: 'hundred.txt',
        regex: 'Line (50|52|54):',
        caseSensitive: true,
        before: 2,
        after: 2
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          file: 'hundred.txt',
          regex: 'Line (50|52|54):',
          caseSensitive: true,
          before: 2,
          after: 2
        })}${NC}`);
        console.log('Output:');
        console.log(result);
        console.log(`${GRAY}Note: Shows how overlapping context is handled${NC}`);
      }
      if (!result.includes('3 matches found')) throw new Error('Should find 3 matches');
      // Let's see what the actual output looks like for overlapping matches
      if (verbose) {
        console.log(`${GRAY}Analysis: Checking how overlapping contexts are displayed${NC}`);
      }
    }
  },

  {
    name: 'RGrep basic',
    fn: async (client) => {
      const result = await client.callTool('RGrep', {
        dir: 'dir1',
        regex: 'console',
        caseSensitive: true
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          dir: 'dir1',
          regex: 'console',
          caseSensitive: true
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('nested.js')) throw new Error('Missing file path');
      // RGrep only shows filenames, not match content
      if (result.includes('dir1/nested.js')) throw new Error('Should not include base dir prefix');
    }
  },

  {
    name: 'RGrep root allowed by default',
    fn: async (client) => {
      const result = await client.callTool('RGrep', {
        dir: '.',
        regex: 'content',
        caseSensitive: true
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          dir: '.',
          regex: 'content',
          caseSensitive: true
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('under .')) throw new Error('RGrep root summary should mention "."');
      if (!result.includes('Files matched under .:')) throw new Error('RGrep root should include the files header');
    }
  },

  {
    name: 'RGrep respects maxFiles',
    fn: async (client) => {
      const result = await client.callTool('RGrep', {
        dir: '.',
        regex: 'content',
        caseSensitive: true,
        maxFiles: 1
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          dir: '.',
          regex: 'content',
          caseSensitive: true,
          maxFiles: 1
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.startsWith('WARNING: RGrep stopped early')) throw new Error('RGrep should start with the early-stop warning');
      const warningIndex = result.indexOf('WARNING: RGrep stopped early');
      const summaryIndex = result.indexOf('1 match found in 1 file');
      if (summaryIndex === -1) throw new Error('RGrep maxFiles should stop after one file');
      if (summaryIndex < warningIndex) throw new Error('RGrep summary should appear after the early-stop warning');
      if (!result.includes('maxFiles=1')) throw new Error('RGrep warning should include the maxFiles value');
      if (!result.includes('Files matched under .:')) throw new Error('RGrep maxFiles output should include the files header');
    }
  },

  {
    name: 'RGrep root blocked with --no-rgrep-root',
    serverArgs: ['--no-rgrep-root'],
    fn: async (client) => {
      try {
        await client.callTool('RGrep', {
          dir: '.',
          regex: 'content',
          caseSensitive: true
        });
        throw new Error('Should have failed on root RGrep with --no-rgrep-root');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        }
        if (!String(e.message).includes('--no-rgrep-root')) {
          throw new Error('Error should mention --no-rgrep-root');
        }
      }
    }
  },

  {
    name: 'RGrep with symlinks shows warnings',
    fn: async (client) => {
      const result = await client.callTool('RGrep', {
        dir: 'test_tree',
        regex: 'content',
        caseSensitive: true
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          dir: 'test_tree',
          regex: 'content',
          caseSensitive: true
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('file.txt')) throw new Error('Missing regular file match');
      // RGrep warnings include the full relative path from base directory
      if (!result.includes('WARNING: test_tree/link_file -> file.txt not followed')) throw new Error('Missing file symlink warning');
      if (!result.includes('WARNING: test_tree/link_dir -> ../dir1')) throw new Error('Missing dir symlink warning');
    }
  },

  {
    name: 'RGrep with directory symlink loop',
    fn: async (client) => {
      // Test warnings without creating new files
      const result = await client.callTool('RGrep', {
        dir: 'loop_base',
        regex: 'findme',
        caseSensitive: true
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          dir: 'loop_base',
          regex: 'findme',
          caseSensitive: true
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      // Should show no matches since we're searching for 'findme' which doesn't exist
      if (!result.includes('0 matches found')) throw new Error('Should indicate no matches');

      // Just check for warnings (can't create test.txt without Write tool)
      if (!result.includes('WARNING: loop_base/loop_dir_a')) throw new Error('Missing loop warning');

      // Should not infinitely recurse
      const warningCount = (result.match(/WARNING:/g) || []).length;
      if (warningCount !== 4) throw new Error(`Expected 4 warnings, got ${warningCount}`);
    }
  },



  {
    name: 'RGrep no match',
    fn: async (client) => {
      const result = await client.callTool('RGrep', {
        dir: 'dir1',
        regex: 'nonexistent_pattern',
        caseSensitive: true
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({
          dir: 'dir1',
          regex: 'nonexistent_pattern',
          caseSensitive: true
        })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('0 matches found')) throw new Error('Should indicate no matches');
    }
  },

  // Security tests
  {
    name: 'Path traversal prevention',
    fn: async (client) => {
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ file: '../../../etc/passwd', start: 0, lines: 100, headOrTail: 'head' })}${NC}`);
      }
      try {
        await client.callTool('Read', { file: '../../../etc/passwd', start: 0, lines: 100, headOrTail: 'head' });
        throw new Error('Should have prevented path traversal');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Correctly blocked: ${e.message}${NC}`);
        }
      }
    }
  },

  {
    name: 'Absolute path prevention',
    fn: async (client) => {
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ file: '/etc/passwd', start: 0, lines: 100, headOrTail: 'head' })}${NC}`);
      }
      try {
        await client.callTool('Read', { file: '/etc/passwd', start: 0, lines: 100, headOrTail: 'head' });
        throw new Error('Should have prevented absolute path');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Correctly blocked: ${e.message}${NC}`);
        }
      }
    }
  },

  // Edge cases
  {
    name: 'Empty directory listing',
    fn: async (client) => {
      const result = await client.callTool('ListDir', { dir: 'empty_dir' });
      if (verbose) {
        console.log(`${GRAY}Tool: ListDir${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'empty_dir' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('0 files and 0 directories')) throw new Error('Should indicate empty directory');
    }
  },

  {
    name: 'Binary file handling',
    fn: async (client) => {
      const result = await client.callTool('Read', { file: 'binary.dat', start: 0, lines: 100, headOrTail: 'head' });
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ file: 'binary.dat', start: 0, lines: 100, headOrTail: 'head' })}${NC}`);
        console.log('Output:');
        console.log(result.substring(0, 100) + '...');
      }
      // Binary content should be hex-escaped as \u00XX
      if (!result.includes('\\u00')) throw new Error('Should show hex escapes for invalid UTF-8');
    }
  },

  {
    name: 'Large file chunking',
    fn: async (client) => {
      // Use existing hundred.txt instead of creating large file
      const result = await client.callTool('Read', { file: 'hundred.txt', start: 0, lines: 10, headOrTail: 'head' });
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ file: 'hundred.txt', start: 0, lines: 10, headOrTail: 'head' })}${NC}`);
        console.log(`${GRAY}Result length: ${result.length} chars${NC}`);
      }
      const lineCount = (result.match(/\n/g) || []).length;
      if (lineCount > 11) throw new Error('Should only return requested lines plus header');
    }
  },

  {
    name: 'Non-existent file error',
    fn: async (client) => {
      try {
        await client.callTool('Read', { file: 'nonexistent.txt', start: 0, lines: 100, headOrTail: 'head' });
        throw new Error('Should have failed');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        }
      }
    }
  },

  {
    name: 'Invalid regex in Grep',
    fn: async (client) => {
      try {
        await client.callTool('Grep', { file: 'file1.txt', regex: '[', caseSensitive: true });
        throw new Error('Should have failed on invalid regex');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        }
      }
    }
  },

  {
    name: 'Regex safety guard - catastrophic pattern',
    fn: async (client) => {
      try {
        await client.callTool('Grep', { file: 'file1.txt', regex: '.*.*', caseSensitive: true, before: 0, after: 0 });
        throw new Error('Should have failed on catastrophic regex');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        }
        if (!String(e.message).includes('catastrophic backtracking')) {
          throw new Error('Error should mention catastrophic backtracking');
        }
      }
    }
  },

  {
    name: 'Regex safety guard - pattern length',
    fn: async (client) => {
      const longPattern = 'a'.repeat(10001);
      try {
        await client.callTool('Grep', { file: 'file1.txt', regex: longPattern, caseSensitive: true, before: 0, after: 0 });
        throw new Error('Should have failed on overly long regex');
      } catch (e) {
        if (verbose) {
          console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        }
        if (!String(e.message).includes('Regex pattern too long')) {
          throw new Error('Error should mention regex length limit');
        }
      }
    }
  },

  {
    name: 'Relative symlink support',
    fn: async (client) => {
      const result = await client.callTool('ListDir', { dir: 'dir2' });
      if (verbose) {
        console.log(`${GRAY}Tool: ListDir${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'dir2' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }
      if (!result.includes('link_relative -> ../dir1')) throw new Error('Should show relative symlink');
    }
  },

  {
    name: 'Find finds files, directories, and symlinks',
    fn: async (client) => {
      // Create a test structure with mixed types
      await fs.promises.mkdir(path.join(TEST_DIR, 'find_test'), { recursive: true });
      await fs.promises.mkdir(path.join(TEST_DIR, 'find_test', 'subdir1'));
      await fs.promises.mkdir(path.join(TEST_DIR, 'find_test', 'subdir2'));
      await fs.promises.writeFile(path.join(TEST_DIR, 'find_test', 'file1.txt'), 'content1');
      await fs.promises.writeFile(path.join(TEST_DIR, 'find_test', 'file2.md'), 'content2');
      await fs.promises.writeFile(path.join(TEST_DIR, 'find_test', 'subdir1', 'nested.js'), 'nested content');
      // Create symlinks
      await fs.promises.symlink('file1.txt', path.join(TEST_DIR, 'find_test', 'link_to_file'));
      await fs.promises.symlink('subdir1', path.join(TEST_DIR, 'find_test', 'link_to_dir'));

      // Test with * glob to find everything
      const result = await client.callTool('Find', { dir: 'find_test', glob: '*' });
      if (verbose) {
        console.log(`${GRAY}Tool: Find${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'find_test', glob: '*' })}${NC}`);
        console.log('Output:');
        console.log(result);
      }

      // Verify files are found
      if (!result.includes('file1.txt')) throw new Error('Find should find regular files');
      if (!result.includes('file2.md')) throw new Error('Find should find all files');

      // Verify directories are found
      if (!result.includes('subdir1/')) throw new Error('Find should find directories (with trailing /)');
      if (!result.includes('subdir2/')) throw new Error('Find should find all directories');

      // Verify symlinks are found and shown with their targets
      if (!result.includes('link_to_file -> file1.txt')) throw new Error('Find should show file symlinks with targets');
      if (!result.includes('link_to_dir -> subdir1')) throw new Error('Find should show directory symlinks with targets');

      // Test with **/* to find nested items too
      const resultRecursive = await client.callTool('Find', { dir: 'find_test', glob: '**/*' });
      if (verbose) {
        console.log(`\n${GRAY}Tool: Find (recursive)${NC}`);
        console.log(`${GRAY}Parameters: ${JSON.stringify({ dir: 'find_test', glob: '**/*' })}${NC}`);
        console.log('Output:');
        console.log(resultRecursive);
      }

      // Verify nested file is found with recursive glob
      if (!resultRecursive.includes('subdir1/nested.js')) throw new Error('Find should find nested files with **/* glob');
    }
  },
  {
    name: 'Default exclusions apply when .mcpignore is missing',
    setup: async () => {
      await removeMcpIgnore();
    },
    fn: async (client) => {
      const result = await client.callTool('ListDir', { dir: '.' });
      if (result.includes('.git/')) throw new Error('Default exclusions should hide .git');
      if (result.includes('node_modules/')) throw new Error('Default exclusions should hide node_modules');
    }
  },

  {
    name: '.mcpignore exclusions apply to all tools',
    setup: async () => {
      await writeMcpIgnore([
        'excluded_dir/',
        'excluded_file.txt',
        'exclude_scope/excluded_child.txt',
        'exclude_scope/excluded_child_dir/'
      ].join('\n'));
    },
    teardown: async () => {
      await removeMcpIgnore();
    },
    fn: async (client) => {
      const listRoot = await client.callTool('ListDir', { dir: '.' });
      if (listRoot.includes('excluded_dir/')) throw new Error('ListDir should skip excluded_dir');
      if (listRoot.includes('excluded_file.txt')) throw new Error('ListDir should skip excluded_file.txt');
      if (listRoot.includes('link_to_excluded')) throw new Error('ListDir should skip symlink to excluded target');
      if (!listRoot.includes('.git/')) throw new Error('ListDir should show .git when .mcpignore exists');

      const listScope = await client.callTool('ListDir', { dir: 'exclude_scope' });
      if (!listScope.includes('visible.txt')) throw new Error('ListDir should show visible.txt');
      if (listScope.includes('excluded_child.txt')) throw new Error('ListDir should skip excluded_child.txt');
      if (listScope.includes('excluded_child_dir/')) throw new Error('ListDir should skip excluded_child_dir');

      const treeScope = await client.callTool('Tree', { dir: 'exclude_scope' });
      if (!treeScope.includes('visible.txt')) throw new Error('Tree should show visible.txt');
      if (treeScope.includes('excluded_child.txt')) throw new Error('Tree should skip excluded_child.txt');
      if (treeScope.includes('excluded_child_dir')) throw new Error('Tree should skip excluded_child_dir');

      const findScope = await client.callTool('Find', { dir: 'exclude_scope', glob: '*' });
      if (!findScope.includes('visible.txt')) throw new Error('Find should show visible.txt');
      if (findScope.includes('excluded_child.txt')) throw new Error('Find should skip excluded_child.txt');
      if (findScope.includes('excluded_child_dir')) throw new Error('Find should skip excluded_child_dir');

      const rgrepRoot = await client.callTool('RGrep', { dir: '.', regex: 'EXCLUDED', caseSensitive: true });
      if (!rgrepRoot.includes('allowed_file.txt')) throw new Error('RGrep should find allowed_file.txt');
      if (rgrepRoot.includes('excluded_file.txt')) throw new Error('RGrep should skip excluded_file.txt');
      if (rgrepRoot.includes('excluded_dir/secret.txt')) throw new Error('RGrep should skip excluded_dir');

      client.errorOutput = '';
      try {
        await client.callTool('Read', { file: 'excluded_file.txt', start: 0, lines: 10, headOrTail: 'head' });
        throw new Error('Read should fail with ENOENT for excluded file');
      } catch (e) {
        if (!String(e.message).includes('ENOENT')) throw new Error('Read should return ENOENT for excluded file');
        if (!client.errorOutput.includes('excluded path')) throw new Error('Read should log excluded path');
      }

      client.errorOutput = '';
      try {
        await client.callTool('Grep', { file: 'excluded_file.txt', regex: 'EXCLUDED', caseSensitive: true, before: 0, after: 0 });
        throw new Error('Grep should fail with ENOENT for excluded file');
      } catch (e) {
        if (!String(e.message).includes('ENOENT')) throw new Error('Grep should return ENOENT for excluded file');
        if (!client.errorOutput.includes('excluded path')) throw new Error('Grep should log excluded path');
      }

      client.errorOutput = '';
      try {
        await client.callTool('ListDir', { dir: 'excluded_dir' });
        throw new Error('ListDir should fail with ENOENT for excluded directory');
      } catch (e) {
        if (!String(e.message).includes('ENOENT')) throw new Error('ListDir should return ENOENT for excluded directory');
        if (!client.errorOutput.includes('excluded path')) throw new Error('ListDir should log excluded path');
      }
    }
  },

  {
    name: '.mcpignore unreadable falls back to defaults',
    setup: async () => {
      await removeMcpIgnore();
      await fs.promises.mkdir(MCPIGNORE_PATH);
    },
    teardown: async () => {
      await removeMcpIgnore();
    },
    fn: async (client) => {
      const result = await client.callTool('ListDir', { dir: '.' });
      if (result.includes('.git/')) throw new Error('Defaults should hide .git when .mcpignore is unreadable');
      if (result.includes('node_modules/')) throw new Error('Defaults should hide node_modules when .mcpignore is unreadable');
      if (!client.errorOutput.includes('Failed to read .mcpignore')) throw new Error('Unreadable .mcpignore should log an error');
    }
  },

  {
    name: 'RGrep default skips binary decode',
    setup: async () => {
      await removeMcpIgnore();
    },
    fn: async (client) => {
      const result = await client.callTool('RGrep', { dir: '.', regex: '\\\\u00ff', caseSensitive: true });
      if (result.includes('binary.dat')) throw new Error('RGrep should not match binary.dat without decode');
    }
  },

  {
    name: 'RGrep decode binary flag matches hex escapes',
    setup: async () => {
      await removeMcpIgnore();
    },
    serverArgs: ['--rgrep-decode-binary'],
    fn: async (client) => {
      const result = await client.callTool('RGrep', { dir: '.', regex: '\\\\u00ff', caseSensitive: true });
      if (!result.includes('binary.dat')) throw new Error('RGrep should match binary.dat with --rgrep-decode-binary');
    }
  }
];

// Display mode tests (verbose only)
const displayTests = [
  {
    name: 'Display ListDir with timestamps',
    fn: async (client) => {
      console.log(`\n${YELLOW}═══ ListDir with timestamps ═══${NC}`);
      const params = {
        dir: 'project',
        showSize: true,
        showLastModified: true,
        showCreated: true
      };
      console.log(`${GRAY}Tool: ListDir${NC}`);
      console.log(`${GRAY}Parameters: ${JSON.stringify(params)}${NC}`);
      const result = await client.callTool('ListDir', params);
      console.log('Output:');
      console.log(result);
      console.log(`${GRAY}Note: Directories show no size, files show size in bytes${NC}`);
    }
  },

  {
    name: 'Display Tree with size',
    fn: async (client) => {
      console.log(`\n${YELLOW}═══ Tree with size ═══${NC}`);
      const params = { dir: 'project', showSize: true };
      console.log(`${GRAY}Tool: Tree${NC}`);
      console.log(`${GRAY}Parameters: ${JSON.stringify(params)}${NC}`);
      const result = await client.callTool('Tree', params);
      console.log('Output:');
      console.log(result);
      console.log(`${GRAY}Note: Size shown only for files, not directories${NC}`);
    }
  },

  {
    name: 'Display Grep with context',
    fn: async (client) => {
      console.log(`\n${YELLOW}═══ Grep with context ═══${NC}`);
      const params = {
        file: 'hundred.txt',
        regex: 'Line 50',
        caseSensitive: true,
        before: 2,
        after: 3
      };
      console.log(`${GRAY}Tool: Grep${NC}`);
      console.log(`${GRAY}Parameters: ${JSON.stringify(params)}${NC}`);
      const result = await client.callTool('Grep', params);
      console.log('Output:');
      console.log(result);
      console.log(`${GRAY}Note: Shows 2 lines before and 3 lines after match${NC}`);
    }
  },

  {
    name: 'Display symlink warnings',
    fn: async (client) => {
      console.log(`\n${YELLOW}═══ Symlink warnings in Tree ═══${NC}`);
      const params = { dir: 'test_tree' };
      console.log(`${GRAY}Tool: Tree${NC}`);
      console.log(`${GRAY}Parameters: ${JSON.stringify(params)}${NC}`);
      const result = await client.callTool('Tree', params);
      console.log('Output:');
      console.log(result);
      console.log(`${GRAY}Note: Symlinks show WARNING instead of being followed${NC}`);
    }
  },

  {
    name: 'Display symlink loop detection',
    fn: async (client) => {
      console.log(`\n${YELLOW}═══ Symlink loop detection ═══${NC}`);
      const params = { dir: 'loop_base' };
      console.log(`${GRAY}Tool: Tree${NC}`);
      console.log(`${GRAY}Parameters: ${JSON.stringify(params)}${NC}`);
      const result = await client.callTool('Tree', params);
      console.log('Output:');
      console.log(result);
      console.log(`${GRAY}Note: Loop symlinks detected and not followed${NC}`);
    }
  }
];

// Main test runner
async function main() {
  console.log(`${BLUE}╔════════════════════════════════════════╗${NC}`);
  console.log(`${BLUE}║   FS-MCP Server Comprehensive Tests   ║${NC}`);
  console.log(`${BLUE}╚════════════════════════════════════════╝${NC}`);

  try {
    await setupTestEnvironment();

    // Run all tests and collect results
    for (const test of tests) {
      const result = await runTest(test.name, test.fn, test.serverArgs, test.setup, test.teardown);
      testResults.push(result);
    }

    // Run display tests in verbose mode
    if (verbose) {
      console.log(`\n${BLUE}═══ Display Tests (Visual Verification) ═══${NC}`);
      const client = new MCPTestClient();
      await client.start();

      for (const test of displayTests) {
        try {
          await test.fn(client);
        } catch (e) {
          console.error(`${RED}Display test failed: ${e.message}${NC}`);
        }
      }

      await client.stop();
    }

    // Summary
    console.log(`\n${BLUE}═══ Test Summary ═══${NC}`);
    console.log(`${GREEN}Passed: ${testsPassed}${NC}`);
    console.log(`${RED}Failed: ${testsFailed}${NC}`);

    if (testsFailed > 0) {
      console.log(`\n${RED}Failed tests:${NC}`);
      testResults.filter(r => !r.passed).forEach(r => {
        console.log(`  ${RED}✗ ${r.name}: ${r.error}${NC}`);
      });
    }

    // Cleanup
    await fs.promises.rm(TEST_DIR, { recursive: true, force: true });
    console.log(`${GRAY}Test directory cleaned up${NC}`);

    process.exit(testsFailed > 0 ? 1 : 0);
  } catch (error) {
    console.error(`${RED}Test setup failed: ${error.message}${NC}`);
    process.exit(1);
  }
}

// Run tests
main().catch(console.error);
