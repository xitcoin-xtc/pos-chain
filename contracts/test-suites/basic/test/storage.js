import { expect, createTestRuntime } from '../../../scripts/solidity-suite-runtime.mjs';
const { ethers } = await createTestRuntime();

describe('Test Storage Contract', function () {
  let storageInstance;
  it('should deploy Storage contract', async function () {
    storageInstance = await ethers.deployContract('Storage');
    await storageInstance.waitForDeployment();
    expect(await storageInstance.getAddress()).to.match(/^0x[0-9a-fA-F]{40}$/);
  });
  it('should successfully store a value', async function () {
    const receipt = await (await storageInstance.store(888)).wait();
    expect(receipt.hash).to.match(/^0x[0-9a-fA-F]{64}$/);
    expect(receipt.status).to.equal(1);
  });
  it('should successfully retrieve a value', async function () {
    expect(await storageInstance.retrieve()).to.equal(888n);
  });
});
