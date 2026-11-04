import assert from 'node:assert/strict';
import { expect, createTestRuntime } from '../../../scripts/solidity-suite-runtime.mjs';
const { ethers } = await createTestRuntime();

describe('OpCodes', function () {
  let contractInstance;
  before(async function () {
    contractInstance = await ethers.deployContract('OpCodes');
    await contractInstance.waitForDeployment();
  });
  it('Should run the majority of opcodes without errors', async function () {
    expect((await (await contractInstance.test()).wait()).status).to.equal(1);
    expect((await (await contractInstance.test_stop()).wait()).status).to.equal(1);
  });
  it('Should throw invalid op code', async function () {
    await assert.rejects(contractInstance.test_invalid(), /invalid opcode|revert|VM Exception/i);
  });
  it('Should revert', async function () {
    await expect(contractInstance.test_revert()).to.revert(ethers);
  });
});
