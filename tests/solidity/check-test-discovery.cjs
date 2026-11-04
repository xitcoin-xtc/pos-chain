// Exercise real manifest discovery without starting an EVM or a chain node.
const assert = require('node:assert/strict');
const { loadTests } = require('./test-helper.js');

assert.deepEqual(loadTests({}).sort(), [
  'basic', 'eip1559', 'exception', 'opcode', 'precompiles', 'revert_cases',
]);
assert.deepEqual(loadTests({ onlyTest: ['basic', 'opcode'] }).sort(), ['basic', 'opcode']);
console.log('All six suites are discoverable after migrating all six suites');

const { spawnSync } = require('node:child_process');
const { join } = require('node:path');
for (const suite of ['precompiles', 'revert_cases']) {
  const result = spawnSync(process.execPath,
    [join(__dirname, '../../contracts/scripts/run-chain-suite.mjs'), suite, '--list'], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  const ordered = JSON.parse(result.stdout);
  assert.ok(ordered.length > 0);
  assert.deepEqual(ordered, [...ordered].sort());
  if (suite === 'precompiles') {
    assert.ok(ordered[0].includes('/1_staking/'));
    assert.ok(ordered.findIndex(path => path.includes('/2_distribution/')) > ordered.findIndex(path => path.includes('/1_staking/')));
    assert.ok(ordered.findIndex(path => path.includes('/5_slashing/')) > ordered.findIndex(path => path.includes('/2_distribution/')));
  }
}
console.log('Chain-specific files retain staking-before-distribution/slashing order');
