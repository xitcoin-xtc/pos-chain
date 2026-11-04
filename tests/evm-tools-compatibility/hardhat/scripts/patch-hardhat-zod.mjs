// Temporary compatibility patch for Hardhat's Zod 3 helper while using fixed Zod 4.
// Fail closed on upstream drift; the package lock pins the reviewed helper.
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';

const helper = new URL('../node_modules/@nomicfoundation/hardhat-zod-utils/', import.meta.url);
const target = new URL('dist/src/index.js', helper);
if (!existsSync(target)) {
  if (existsSync(new URL('../node_modules/hardhat/package.json', import.meta.url))) {
    throw new Error('Hardhat is installed but its reviewed Zod helper is missing');
  }
  // npm ci --omit=dev does not install this development-only tool.
  process.exit(0);
}
const version = JSON.parse(readFileSync(new URL('package.json', helper), 'utf8')).version;
if (version !== '3.0.6') throw new Error(`Unreviewed Hardhat Zod helper version: ${version}`);
const before = readFileSync(target, 'utf8');
const originalSha256 = 'ed7daa506d8a6283b4071036f866168dddf831b5bb39f1735d12138eb3bbbb48';
const changes = [
  ['return result.error.errors.map((issue) => ({', 'return result.error.issues.map((issue) => ({'],
  ['errorMap: () => ({\n        message: errorMessage,\n    }),', 'error: () => errorMessage,'],
];
let original = before;
for (const [oldText, newText] of changes) {
  if (original.includes(newText)) original = original.replace(newText, oldText);
}
if (createHash('sha256').update(original).digest('hex') !== originalSha256) {
  throw new Error('Hardhat Zod helper differs from the reviewed source; patch refused');
}
let patched = original;
for (const [oldText, newText] of changes) {
  if (patched.split(oldText).length !== 2) throw new Error('Ambiguous Hardhat compatibility patch');
  patched = patched.replace(oldText, newText);
}
if (before !== patched) writeFileSync(target, patched);
