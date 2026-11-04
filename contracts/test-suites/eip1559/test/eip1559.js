import { expect, createTestRuntime } from '../../../scripts/solidity-suite-runtime.mjs';
const { ethers } = await createTestRuntime();

describe('Transaction', function () {
  it('should send a transaction with EIP-1559 flag', async function () {
    const [sender, receiver] = await ethers.getSigners();
    const tx = await sender.sendTransaction({ to: receiver.address, value: 10000000n, gasLimit: 21000n, type: 2 });
    const receipt = await tx.wait();
    expect(receipt.status).to.equal(1);
    expect(receipt.type).to.equal(2);
    expect((await ethers.provider.getTransaction(tx.hash)).type).to.equal(2);
  });
});
