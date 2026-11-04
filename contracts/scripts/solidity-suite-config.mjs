import { defineConfig } from 'hardhat/config';
import ethers from '@nomicfoundation/hardhat-ethers';
import matchers from '@nomicfoundation/hardhat-ethers-chai-matchers';
import mocha from '@nomicfoundation/hardhat-mocha';
import { fileURLToPath } from 'node:url';

// All six Solidity suites share the same audited toolchain.
export function soliditySuiteConfig(suite, version) {
  const directory = fileURLToPath(new URL(`../test-suites/${suite}/`, import.meta.url));
  return defineConfig({
    plugins: [ethers, matchers, mocha],
    solidity: {
      version,
      settings: {
        optimizer: { enabled: false, runs: 200 },
        evmVersion: version === '0.5.17' ? 'istanbul' : 'paris',
      },
    },
    networks: {
      cosmos: {
        type: 'http',
        chainType: 'l1',
        url: 'http://127.0.0.1:8545',
        chainId: Number(process.env.EXPECTED_EVM_CHAIN_ID || 262144),
        accounts: 'remote',
        gas: 5000000,
        gasPrice: 1000000000,
      },
    },
    paths: {
      sources: `${directory}contracts`, tests: `${directory}test`,
      cache: `${directory}cache`, artifacts: `${directory}artifacts`,
    },
    test: { mocha: { timeout: 120000, failZero: true, forbidOnly: true, forbidPending: true } },
  });
}

export function chainSuiteConfig(suite) {
  const config = soliditySuiteConfig(suite, '0.8.18');
  config.solidity = {
    compilers: [
      { version: '0.8.18', settings: { optimizer: { enabled: false, runs: 200 }, evmVersion: 'paris' } },
      { version: '0.4.22', settings: { optimizer: { enabled: false, runs: 200 }, evmVersion: 'byzantium' } },
    ],
  };
  // The existing protected governance policy intentionally leaves 12 legacy
  // lifecycle cases pending. All other suites still forbid pending tests.
  config.test.mocha.forbidPending = suite !== 'precompiles';
  return config;
}
