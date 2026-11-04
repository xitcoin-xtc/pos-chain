import { expect, createTestRuntime } from '../../../scripts/solidity-suite-runtime.mjs';
const { ethers } = await createTestRuntime();

describe('Test EventTest Contract', function () {
  let eventInstance;
  it('should deploy EventTest contract', async function () {
    eventInstance = await ethers.deployContract('EventTest');
    await eventInstance.waitForDeployment();
    expect(await eventInstance.getAddress()).to.match(/^0x[0-9a-fA-F]{40}$/);
  });
  it('should emit events', async function () {
    const tx = await eventInstance.storeWithEvent(888);
    await expect(tx).to.emit(eventInstance, 'ValueStored1').withArgs(888n);
    await expect(tx).to.emit(eventInstance, 'ValueStored2').withArgs('TestMsg', 888n);
    await expect(tx).to.emit(eventInstance, 'ValueStored3').withArgs('TestMsg', 888n, 888n);
  });
});
