// Exercise the real consumers affected by security overrides, on a disposable runner.
const assert = require('node:assert/strict');
const http = require('node:http');
const { dirname } = require('node:path');
const { once } = require('node:events');

async function main() {
  const Handlebars = require('handlebars');
  assert.equal(Handlebars.VERSION, '4.7.10');
  const templates = Handlebars.create();
  templates.registerPartial('item', '[{{name}}]');
  assert.equal(templates.compile('{{title}}:{{#each items}}{{> item}}{{/each}}')({
    title: '<safe>', items: [{ name: 'one' }, { name: 'two' }],
  }), '&lt;safe&gt;:[one][two]');
  // Synthetic probes only: no operating-system command or external input.
  const lookup = templates.compile('{{lookup (lookup fn "__proto__") "constructor"}}');
  assert.equal(lookup({ fn() {} }, { allowProtoMethodsByDefault: true }), '');
  const invalidAst = templates.parse('{{#if ok}}safe{{/if}}');
  invalidAst.body[0].program.blockParams = { length: '(globalThis.__handlebarsSecurityFixture = 1, 0)' };
  assert.throws(() => templates.compile(invalidAst)({ ok: true }));
  assert.throws(() => templates.precompile(invalidAst));
  assert.equal(globalThis.__handlebarsSecurityFixture, undefined);

  // Real legacy consumers, with disposable in-memory keys and fake browser storage.
  const keys = require('libp2p-crypto').keys;
  const rsa = await keys.generateKeyPair('RSA', 2048);
  const restored = await keys.unmarshalPrivateKey(keys.marshalPrivateKey(rsa));
  const message = Buffer.from('synthetic-library-compatibility');
  const signature = await restored.sign(message);
  assert.equal(await restored.public.verify(message, signature), true);
  assert.equal(await restored.public.verify(Buffer.from('changed'), signature), false);

  const withTimeout = require('ipfs-core-utils/src/with-timeout-option');
  assert.equal(await withTimeout(async () => 'ok')({ timeout: '2 seconds' }), 'ok');
  await assert.rejects(withTimeout(() => new Promise(() => {}))({ timeout: '10 ms' }),
    error => error.name === 'TimeoutError');

  const jar = new Map();
  const writes = [];
  global.document = {
    get cookie() { return [...jar].map(([key, value]) => `${key}=${value}`).join('; '); },
    set cookie(value) {
      writes.push(value);
      const [pair] = value.split(';');
      const position = pair.indexOf('=');
      const key = pair.slice(0, position);
      const expiry = /(?:^|;\s*)expires=([^;]+)/i.exec(value);
      if (expiry && Date.parse(expiry[1]) < Date.now()) jar.delete(key);
      else jar.set(key, pair.slice(position + 1));
    },
  };
  try {
    const CookieStorage = require('amazon-cognito-identity-js').CookieStorage;
    const storage = new CookieStorage({ domain: 'synthetic.invalid', sameSite: 'strict' });
    assert.equal(storage.setItem('fixture', 'safe value'), 'safe value');
    assert.equal(storage.getItem('fixture'), 'safe value');
    storage.clear();
    assert.equal(storage.getItem('fixture'), null);
    const Cookies = require('js-cookie');
    Cookies.set('fixture', 'value', JSON.parse('{"__proto__":{"domain":"unexpected.invalid"}}'));
    assert.equal(writes.at(-1).includes('unexpected.invalid'), false);
    assert.equal(Object.prototype.domain, undefined);
  } finally { delete global.document; }

  const AdmZip = require('adm-zip');
  const zip = new AdmZip();
  zip.addFile('synthetic.txt', Buffer.from('safe archive'));
  assert.equal(new AdmZip(zip.toBuffer()).readAsText('synthetic.txt'), 'safe archive');
  const codecSemver = require(require.resolve('semver', {
    paths: [dirname(require.resolve('@truffle/codec/package.json'))],
  }));
  assert.equal(codecSemver.satisfies('0.8.30', '>=0.8.0 <0.9.0'), true);
  assert.equal(codecSemver.satisfies('0.7.6', '>=0.8.0 <0.9.0'), false);

  const { mergeDeep } = require('@graphql-tools/utils');
  assert.deepEqual(mergeDeep([{ a: { b: 1 } }, { a: { c: 2 } }]), { a: { b: 1, c: 2 } });
  const merged = mergeDeep([{}, JSON.parse('{"__proto__":{"securityFixturePollution":true}}')]);
  assert.equal(merged.securityFixturePollution, undefined);
  assert.equal(Object.prototype.securityFixturePollution, undefined);

  const fetch = require('fetch-ponyfill')().fetch;
  const server = http.createServer((request, response) => {
    assert.equal(request.method, 'POST');
    assert.equal(request.headers['content-type'], 'application/json');
    response.setHeader('content-type', 'application/json');
    response.end('{"synthetic":true}');
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  try {
    const result = await fetch(`http://127.0.0.1:${server.address().port}`, {
      method: 'POST', headers: { 'content-type': 'application/json' }, body: '{"test":true}',
    });
    assert.equal(result.status, 200);
    assert.deepEqual(await result.json(), { synthetic: true });
    const endpoint = `http://127.0.0.1:${server.address().port}`;
    const undici = require('undici');
    const request = await undici.request(endpoint, {
      method: 'POST', headers: { 'content-type': 'application/json' }, body: '{"test":true}',
    });
    assert.equal(request.statusCode, 200);
    assert.deepEqual(await request.body.json(), { synthetic: true });
    const FilecoinRPC = require('@zondax/filecoin-signing-tools/src/utils/index.js');
    const rpc = new FilecoinRPC({ url: endpoint, token: 'synthetic-not-a-credential' });
    assert.deepEqual(await rpc.getNonce('synthetic-address'), { synthetic: true });
  } finally { await new Promise(resolve => server.close(resolve)); }

  const Ws = require(require.resolve('ws', { paths: [dirname(require.resolve('eth-lib/package.json'))] }));
  const wss = new Ws.Server({ host: '127.0.0.1', port: 0 });
  await once(wss, 'listening');
  wss.on('connection', socket => socket.on('message', data => socket.send(data)));
  const socket = new Ws(`ws://127.0.0.1:${wss.address().port}`);
  try {
    await once(socket, 'open');
    const message = once(socket, 'message');
    socket.send('synthetic-compatibility-only');
    assert.equal(String((await message)[0]), 'synthetic-compatibility-only');
  } finally {
    socket.terminate();
    for (const client of wss.clients) client.terminate();
    await new Promise(resolve => wss.close(resolve));
  }

  const { connect } = require('@truffle/db');
  const db = connect({ adapter: { name: 'memory' } });
  const added = await db.execute(`mutation { sourcesAdd(input: {sources: [
    {contents: "contract SecurityCompatibilityFixture {}", sourcePath: "Synthetic.sol"}
  ]}) { sources { id contents sourcePath } } }`);
  assert.equal(added.errors, undefined, JSON.stringify(added.errors));
  const source = added.data.sourcesAdd.sources[0];
  assert.equal(source.sourcePath, 'Synthetic.sol');
  const queried = await db.execute('query($id: ID!) { source(id: $id) { id contents sourcePath } }', { id: source.id });
  assert.equal(queried.errors, undefined, JSON.stringify(queried.errors));
  assert.equal(queried.data.source.id, source.id);
  assert.equal(queried.data.source.contents, 'contract SecurityCompatibilityFixture {}');
  console.log('Template guards, RSA roundtrip, IPFS timeouts, synthetic cookies, ZIP, compiler ranges, HTTP clients, GraphQL DB and WebSocket passed');
}

const deadline = setTimeout(() => { console.error('Compatibility check timed out'); process.exit(1); }, 30000);
main().then(() => { clearTimeout(deadline); process.exit(0); }, error => { console.error(error); process.exit(1); });
