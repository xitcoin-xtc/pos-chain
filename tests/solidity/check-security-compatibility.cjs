// Verify the remaining consumers and reject reintroduction of the retired server.
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const http = require('node:http');
const { dirname, join } = require('node:path');
const { once } = require('node:events');

async function main() {
  const lock = readFileSync(join(__dirname, 'yarn.lock'), 'utf8');
  assert.doesNotMatch(lock, /^(?:"?truffle@|"?@truffle\/db@|"?apollo-server[^\n]*@)/m);
  for (const name of ['truffle', '@truffle/db', 'apollo-server', 'apollo-server-core']) {
    assert.throws(() => require.resolve(name), { code: 'MODULE_NOT_FOUND' });
  }

  for (const name of ['hardhat', 'elliptic', 'request', 'web3-core-subscriptions']) {
    assert.throws(() => require.resolve(name), { code: 'MODULE_NOT_FOUND' });
  }
  const { createRequire } = require('node:module');
  const tooling = createRequire(join(__dirname, '../../contracts/package.json'));
  assert.equal(JSON.parse(readFileSync(join(__dirname, '../../contracts/node_modules/hardhat/package.json'))).version, '3.14.0');
  const ethers = tooling('ethers');
  const wallet = ethers.Wallet.createRandom();
  const message = 'Offline synthetic security regression';
  assert.equal(ethers.verifyMessage(message, await wallet.signMessage(message)), wallet.address);
  const Ws = tooling(tooling.resolve('ws', { paths: [dirname(tooling.resolve('ethers'))] }));
  const wss = new Ws.Server({ host: '127.0.0.1', port: 0 });
  wss.on('connection', socket => socket.on('message', data => {
    const request = JSON.parse(String(data));
    const respond = item => {
      assert.ok(['eth_chainId', 'eth_blockNumber'].includes(item.method));
      return { jsonrpc: '2.0', id: item.id, result: item.method === 'eth_chainId' ? '0x1' : '0x2a' };
    };
    socket.send(JSON.stringify(Array.isArray(request) ? request.map(respond) : respond(request)));
  }));
  await once(wss, 'listening');
  const provider = new ethers.WebSocketProvider(`ws://127.0.0.1:${wss.address().port}`);
  try {
    assert.equal(await provider.getBlockNumber(), 42);
    assert.equal((await provider.getNetwork()).chainId, 1n);
  } finally {
    await provider.destroy();
    for (const client of wss.clients) client.terminate();
    await new Promise(resolve => wss.close(resolve));
  }
  console.log('Retired Truffle/Apollo/Hardhat2 and vulnerable crypto/HTTP clients absent; modern signer roundtrip and real ethers WebSocket passed');
}

const deadline = setTimeout(() => { console.error('Compatibility check timed out'); process.exit(1); }, 30000);
main().then(() => { clearTimeout(deadline); process.exit(0); }, error => { console.error(error); process.exit(1); });
