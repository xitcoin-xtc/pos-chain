// Preserve Web3's validation errors while using fixed Zod4.
// Both shipped entry points are pinned by version and original source hash.
const { createHash } = require('node:crypto');
const { readFileSync, writeFileSync } = require('node:fs');
const { join } = require('node:path');
const root = join(__dirname, '../node_modules/web3-validator');
if (JSON.parse(readFileSync(join(root, 'package.json'))).version !== '2.0.6') throw new Error('Unreviewed Web3 validator version');
for (const [file, hash, z, formats] of [
  ['lib/commonjs/validator.js', '9e202a6da24e6c06840d6c8bd2d89f7764ca44e079fbb165fc477c2f5ea39e98', 'zod_1.z', 'formats_js_1.default'],
  ['lib/esm/validator.js', 'ed09286406b0317c371d62da80d7c3a45c3ad561e668e5fca3b6a9066150f720', 'z', 'formats'],
]) {
  const path = join(root, file);
  const old = `return ${z}.any().refine(${formats}[schema.format], (value) => ({
            params: { value, format: schema.format },
        }));`;
  const replacement = `return ${z}.any().superRefine((value, ctx) => {
            if (!${formats}[schema.format](value)) ctx.addIssue({
                code: 'custom', params: { value, format: schema.format },
            });
        });`;
  const text = readFileSync(path, 'utf8');
  const original = text.includes(replacement) ? text.replace(replacement, old) : text;
  if (createHash('sha256').update(original).digest('hex') !== hash || original.split(old).length !== 2) throw new Error('Web3 validator source drift: '+file);
  const patched = original.replace(old, replacement);
  if (text !== patched) writeFileSync(path, patched);
}
