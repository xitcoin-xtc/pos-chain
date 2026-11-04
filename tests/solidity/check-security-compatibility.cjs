// Exercise the real consumers affected by security overrides, on a disposable runner.
const assert = require('node:assert/strict');
const http = require('node:http');
const { dirname } = require('node:path');
const { once } = require('node:events');

async function main() {
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
  console.log('GraphQL merge safety, Truffle DB source roundtrip, HTTP ponyfill and legacy WebSocket passed');
}

const deadline = setTimeout(() => { console.error('Compatibility check timed out'); process.exit(1); }, 30000);
main().then(() => { clearTimeout(deadline); process.exit(0); }, error => { console.error(error); process.exit(1); });
