import { expect, createTestRuntime } from '../../../scripts/solidity-suite-runtime.mjs';
const { ethers } = await createTestRuntime();

describe('TestRevert', function () {
  let revert;
  beforeEach(async function () {
    revert = await ethers.deployContract('TestRevert');
    await revert.waitForDeployment();
  });
  it('should revert', async function () {
    await (await revert.try_set(10)).wait();
    expect(await revert.query_a()).to.equal(0n);
    expect(await revert.query_b()).to.equal(10n);
    expect(await revert.query_c()).to.equal(10n);
    await (await revert.set(10)).wait();
    expect(await revert.query_a()).to.equal(10n);
  });
});
