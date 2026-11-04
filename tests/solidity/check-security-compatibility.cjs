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

  const Handlebars = require('handlebars');
  assert.equal(Handlebars.VERSION, '4.7.10');
  const templates = Handlebars.create();
  templates.registerPartial('item', '[{{name}}]');
  assert.equal(templates.compile('{{title}}:{{#each items}}{{> item}}{{/each}}')({
    title: '<safe>', items: [{ name: 'one' }, { name: 'two' }],
  }), '&lt;safe&gt;:[one][two]');
  const lookup = templates.compile('{{lookup (lookup fn "__proto__") "constructor"}}');
  assert.equal(lookup({ fn() {} }, { allowProtoMethodsByDefault: true }), '');
  const invalidAst = templates.parse('{{#if ok}}safe{{/if}}');
  invalidAst.body[0].program.blockParams = { length: '(globalThis.__handlebarsSecurityFixture = 1, 0)' };
  assert.throws(() => templates.compile(invalidAst)({ ok: true }));
  assert.throws(() => templates.precompile(invalidAst));
  assert.equal(globalThis.__handlebarsSecurityFixture, undefined);

  const axios = require('axios');
  assert.equal(axios.VERSION, '1.21.0');
  let visitorCalls = 0;
  Object.defineProperty(Object.prototype, 'visitor', {
    configurable: true, value() { visitorCalls++; },
  });
  try { axios.toFormData({ field: 'synthetic' }); }
  finally { delete Object.prototype.visitor; }
  assert.equal(visitorCalls, 0);

  const server = http.createServer((request, response) => {
    assert.equal(request.method, 'POST');
    response.setHeader('content-type', 'application/json');
    response.end('{"synthetic":true}');
  });
  let proxyCalls = 0;
  const proxyTrap = http.createServer((request, response) => {
    proxyCalls++;
    response.setHeader('content-type', 'application/json');
    response.end('{"unexpectedProxy":true}');
  });
  server.listen(0, '127.0.0.1');
  proxyTrap.listen(0, '127.0.0.1');
  await Promise.all([once(server, 'listening'), once(proxyTrap, 'listening')]);
  try {
    const client = axios.create({ baseURL: `http://127.0.0.1:${server.address().port}` });
    client.interceptors.request.use(config => ({ ...config, headers: { ...config.headers } }));
    Object.defineProperty(Object.prototype, 'proxy', {
      configurable: true,
      value: { protocol: 'http', host: '127.0.0.1', port: proxyTrap.address().port },
    });
    try {
      assert.deepEqual((await client.post('/', { test: true })).data, { synthetic: true });
      assert.equal(proxyCalls, 0);
    } finally { delete Object.prototype.proxy; }
  } finally {
    await Promise.all([new Promise(resolve => server.close(resolve)), new Promise(resolve => proxyTrap.close(resolve))]);
  }

  const ethers = require('ethers');
  const Ws = require(require.resolve('ws', { paths: [dirname(require.resolve('ethers'))] }));
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
  console.log('Retired Truffle/Apollo absent; template guards, Axios pollution guards and real ethers WebSocket passed');
}

const deadline = setTimeout(() => { console.error('Compatibility check timed out'); process.exit(1); }, 30000);
main().then(() => { clearTimeout(deadline); process.exit(0); }, error => { console.error(error); process.exit(1); });
