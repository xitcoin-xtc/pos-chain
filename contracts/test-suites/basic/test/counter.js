import { expect, createTestRuntime } from '../../../scripts/solidity-suite-runtime.mjs';
const { ethers } = await createTestRuntime();

describe('Counter', function () {
  let counter;
  let signers;
  beforeEach(async function () {
    signers = (await ethers.getSigners()).slice(0, 3);
    counter = await ethers.deployContract('Counter');
    await counter.waitForDeployment();
  });
  it('should add', async function () {
    const balances = await Promise.all(signers.map(signer => ethers.provider.send('eth_getBalance', [signer.address, 'latest'])));
    for (const [index, signer] of signers.entries()) {
      await (await counter.connect(signer).add()).wait();
      expect(await counter.getCounter()).to.equal(BigInt(index + 1));
      expect(await ethers.provider.send('eth_getBalance', [signer.address, 'latest'])).not.to.equal(balances[index]);
    }
  });
  it('should subtract', async function () {
    await (await counter.add()).wait();
    expect(await counter.getCounter()).to.equal(1n);
    const receipt = await (await counter.subtract()).wait();
    expect(await counter.getCounter()).to.equal(0n);
    const first = counter.interface.parseLog(receipt.logs[0]);
    expect(first.name).to.equal('Changed');
    expect(first.args.counter).to.equal(0n);
    expect(await counter.queryFilter('*', 1, 'latest')).to.have.length(3);
    expect(await counter.queryFilter(counter.filters.Changed(), 1, 'latest')).to.have.length(2);
    await expect(counter.subtract()).to.be.revertedWith('COUNTER_TOO_LOW');
  });
});
