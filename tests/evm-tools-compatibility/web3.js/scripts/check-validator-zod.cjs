const assert = require('node:assert/strict');
const { test } = require('node:test');
const { join, dirname } = require('node:path');
const { Web3 } = require('web3');
const { validator } = require('web3-validator');
const root = dirname(require.resolve('web3-validator'));
const { Validator } = require(join(root, 'validator.js'));
const formats = require(join(root, 'formats.js')).default;

test('Web3 validates valid values and retains field/format details for invalid values', () => {
  validator.validate(['uint8', 'address'], [7, '0x0000000000000000000000000000000000000001']);
  const errors = validator.validate(['uint8', 'address'], [-1, 'invalid'], { silent: true });
  assert.equal(errors.length, 2);
  assert.equal(errors[0].params.value, -1);
  assert.match(errors[0].message, /uint8/);
  assert.match(errors[1].message, /address/);
  assert.throws(() => validator.validate(['uint8'], [-1]), /uint8/);
});

test('the real Web3 validator rejects oversized bounded arrays before element validation', () => {
  let calls = 0;
  formats.securityFixture = value => { calls++; return typeof value === 'string'; };
  try {
    const schema = { type: 'array', maxItems: 2, items: { type: 'string', format: 'securityFixture' } };
    const v = Validator.factory();
    assert.equal(v.validate(schema, Array(1000).fill('synthetic'), { silent: true })[0].keyword, 'maxItems');
    assert.equal(calls, 0);
    assert.equal(v.validate(schema, ['one', 'two']), undefined);
    assert.equal(calls, 2);
  } finally { delete formats.securityFixture; }
});

test('the real Web3 ABI coder, contract encoder and signer preserve their APIs', async () => {
  const web3 = new Web3();
  assert.equal(web3.eth.abi.decodeParameter('uint256', web3.eth.abi.encodeParameter('uint256', 123)), 123n);
  assert.throws(() => web3.eth.abi.encodeParameter('uint8', -1));
  const abi = [{ type: 'function', name: 'transfer', inputs: [{ type: 'address', name: 'to' }, { type: 'uint256', name: 'amount' }], outputs: [{ type: 'bool' }], stateMutability: 'nonpayable' }];
  const contract = new web3.eth.Contract(abi);
  assert.match(contract.methods.transfer('0x0000000000000000000000000000000000000001', 7).encodeABI(), /^0xa9059cbb/);
  assert.throws(() => contract.methods.transfer('invalid', 7).encodeABI(), /address/);
  const wallet = web3.eth.accounts.create();
  const signature = wallet.sign('Offline synthetic compatibility check');
  assert.equal(web3.eth.accounts.recover('Offline synthetic compatibility check', signature.signature), wallet.address);
});
