// Exercise real manifest discovery without starting an EVM or a chain node.
const assert = require('node:assert/strict');
const { loadTests } = require('./test-helper.js');

assert.deepEqual(loadTests({}).sort(), [
  'basic', 'eip1559', 'exception', 'opcode', 'precompiles', 'revert_cases',
]);
assert.deepEqual(loadTests({ onlyTest: ['basic', 'opcode'] }).sort(), ['basic', 'opcode']);
console.log('All six suites are discoverable after moving the four generic suites');
