import { defineConfig } from 'hardhat/config';
import ethers from '@nomicfoundation/hardhat-ethers';
import matchers from '@nomicfoundation/hardhat-ethers-chai-matchers';
import mocha from '@nomicfoundation/hardhat-mocha';
export default defineConfig({
  plugins: [ethers, matchers, mocha],
  solidity: { version: '0.8.28', settings: { optimizer: { enabled: true, runs: 200 }, evmVersion: 'cancun' } },
  networks: {
    localhost: { type: 'http', chainType: 'l1', url: 'http://127.0.0.1:8545',
      chainId: Number(process.env.EXPECTED_EVM_CHAIN_ID || 262144), accounts: 'remote' },
  },
  test: { mocha: { timeout: 120000, failZero: true, forbidOnly: true, forbidPending: true } },
});
