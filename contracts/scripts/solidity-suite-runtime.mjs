export { expect } from 'chai';

export async function createTestRuntime() {
  const { network } = await import('hardhat');
  return network.create();
}
