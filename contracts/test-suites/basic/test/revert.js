import { expect, createTestRuntime } from '../../../scripts/solidity-suite-runtime.mjs';
const { ethers } = await createTestRuntime();

describe('Test EVM Revert', function () {
  let storageInstance;
  it('should deploy Storage contract', async function () {
    storageInstance = await ethers.deployContract('Storage');
    await storageInstance.waitForDeployment();
    expect(await storageInstance.getAddress()).to.match(/^0x[0-9a-fA-F]{40}$/);
  });
  it('should revert when call `shouldRevert()`', async function () {
    await expect(storageInstance.shouldRevert()).to.be.reverted;
  });
});
