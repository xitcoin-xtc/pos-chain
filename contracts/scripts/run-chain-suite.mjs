import { readdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const [suite, ...options] = process.argv.slice(2);
if (!['precompiles', 'revert_cases'].includes(suite)) throw new Error('Unknown chain suite');
const directory = fileURLToPath(new URL(`../test-suites/${suite}/test/`, import.meta.url));
function files(path) {
  return readdirSync(path, { withFileTypes: true }).flatMap(entry =>
    entry.isDirectory() ? files(join(path, entry.name)) :
      entry.isFile() && entry.name.endsWith('.js') ? [join(path, entry.name)] : []);
}
// These scenarios intentionally build staking fixtures before distribution and
// slashing assertions. Preserve Hardhat2's lexicographic discovery order.
const ordered = files(directory).sort();
if (ordered.length === 0) throw new Error('No chain test files discovered');
if (options.length === 1 && options[0] === '--list') {
  console.log(JSON.stringify(ordered));
} else {
  const cli = fileURLToPath(new URL('../node_modules/hardhat/dist/src/cli.js', import.meta.url));
  const config = fileURLToPath(new URL(`../solidity-suites/${suite}.config.mjs`, import.meta.url));
  const result = spawnSync(process.execPath,
    [cli, '--config', config, 'test', 'mocha', ...ordered, ...options], { stdio: 'inherit' });
  if (result.error) throw result.error;
  process.exit(result.status ?? 1);
}
