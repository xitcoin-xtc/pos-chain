import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { z } from 'zod';
import { incompatibleFieldType, resolvedConfigurationVariableSchema,
  sensitiveUrlSchema, validateUserConfigZodType } from '@nomicfoundation/hardhat-zod-utils';

test('oversized bounded collections stop before validating their elements', async () => {
  for (const kind of ['array', 'exact_array', 'set', 'async_array']) {
    let calls = 0;
    const element = z.number().refine(() => { calls++; return true; });
    let schema = kind === 'set' ? z.set(element).max(2) : z.array(element);
    if (kind === 'exact_array') schema = schema.length(2);
    else if (kind !== 'set') schema = schema.max(2);
    const input = Array.from({ length: 1000 }, (_, i) => i);
    const data = kind === 'set' ? new Set(input) : input;
    const result = kind === 'async_array' ? await schema.safeParseAsync(data) : schema.safeParse(data);
    assert.equal(result.success, false);
    assert.equal(calls, 0, kind);
    assert.equal(schema.safeParse(kind === 'set' ? new Set([1, 2]) : [1, 2]).success, true);
  }
});

test('Hardhat validates records, configuration functions, URLs and forbidden fields', () => {
  const schema = z.object({ headers: z.record(z.string()), forbidden: incompatibleFieldType('Forbidden field') });
  assert.deepEqual(validateUserConfigZodType({ headers: { Accept: 'application/json' } }, schema), []);
  assert.ok(validateUserConfigZodType({ headers: { Accept: 1 } }, schema).length > 0);
  assert.deepEqual(validateUserConfigZodType({ headers: {}, forbidden: true }, schema),
    [{ path: ['forbidden'], message: 'Forbidden field' }]);
  assert.equal(sensitiveUrlSchema.safeParse('https://example.invalid').success, true);
  assert.equal(sensitiveUrlSchema.safeParse('invalid URL').success, false);
  assert.equal(resolvedConfigurationVariableSchema.safeParse({ _type: 'ResolvedConfigurationVariable',
    get() {}, getUrl() {}, getBigInt() {}, getHexString() {} }).success, true);
  assert.equal(resolvedConfigurationVariableSchema.safeParse({ _type: 'ResolvedConfigurationVariable',
    get: 'invalid', getUrl() {}, getBigInt() {}, getHexString() {} }).success, false);
});

test('real Hardhat CLI rejects invalid configuration without a parser crash', () => {
  const root = fileURLToPath(new URL('../', import.meta.url));
  const folder = mkdtempSync(join(root, '.hardhat-zod-check-'));
  try {
    const configurations = [
      { solidity: { version: 12 } },
      { networks: { invalid: { type: 'http', url: 17 } } },
      { networks: { invalid: { type: 'http', url: 'https://example.invalid', accounts: 'invalid' } } },
    ];
    for (const config of configurations) {
      const path = join(folder, 'hardhat.config.mjs');
      writeFileSync(path, `export default ${JSON.stringify(config)};\n`);
      const result = spawnSync(process.execPath, [join(root, 'node_modules/hardhat/dist/src/cli.js'),
        'compile', '--config', path], { cwd: root, encoding: 'utf8', timeout: 20000 });
      assert.equal(result.error, undefined);
      assert.notEqual(result.status, 0);
      const output = result.stdout + result.stderr;
      assert.match(output, /HHE15/);
      assert.doesNotMatch(output, /TypeError|Cannot read properties/);
    }
  } finally { rmSync(folder, { recursive: true, force: true }); }
});
