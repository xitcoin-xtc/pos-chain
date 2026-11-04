#!/usr/bin/env python3
"""Collect fresh, versioned binary evidence; never renew security acceptance."""
import hashlib
import json
import pathlib
import shutil
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / 'fresh-versioned-evidence'
MODULE = 'github.com/xitcoin-org/pos-chain/evmd'
HARNESS = ROOT / 'fresh-versioned-harness'
OUT.mkdir(exist_ok=True)
RESULTS = {}


def run(label, args, cwd=ROOT):
    with (OUT / (label + '.log')).open('w') as stream, (OUT / (label + '.stderr')).open('w') as errors:
        result = subprocess.run(args, cwd=cwd, stdout=stream,
                                stderr=errors, timeout=3600)
    RESULTS[label] = {'command': args, 'exit_code': result.returncode}
    (OUT / 'results.json').write_text(json.dumps(RESULTS, indent=2))
    print(label, result.returncode, flush=True)
    result.check_returncode()
    return (OUT / (label + '.log')).read_text()


def objects(raw):
    decoder = json.JSONDecoder()
    while raw.strip():
        raw = raw.lstrip()
        value, end = decoder.raw_decode(raw)
        yield value
        raw = raw[end:]


def graph(raw):
    result = {}
    for module in objects(raw):
        if module.get('Main') or module['Path'] == MODULE:
            continue
        replacement = module.get('Replace', {})
        if not module.get('Version') or (replacement and not replacement.get('Version')):
            raise RuntimeError('Unversioned graph entry: ' + module['Path'])
        result[module['Path']] = (module['Version'], replacement.get('Path'),
                                  replacement.get('Version'))
    return result


def collect():
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    original_locks = {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest()
                      for name in ('go.mod', 'go.sum', 'evmd/go.mod', 'evmd/go.sum')}
    (OUT / 'source.json').write_text(json.dumps({'head': head, 'locks': original_locks}, indent=2))
    run('toolchain', ['go', 'version'])
    baseline = run('baseline-modules', ['go', 'list', '-m', '-json', 'all'], ROOT / 'evmd')
    run('fork-provenance', ['python3', str(ROOT / 'scripts/verify-go-fork-provenance.py')], ROOT / 'evmd')
    resolved = json.loads(run('published-module', ['go', 'list', '-m', '-json', MODULE + '@' + head]))
    if resolved.get('Origin', {}).get('Hash') != head:
        raise RuntimeError('Published module does not resolve to the requested commit')
    HARNESS.mkdir()
    for name in ('go.mod', 'go.sum'):
        shutil.copyfile(ROOT / 'evmd' / name, HARNESS / name)
    run('harness', ['go', 'mod', 'edit', '-module=xitcoin.invalid/security-review',
                    '-require=' + MODULE + '@' + resolved['Version']], HARNESS)
    binary = HARNESS / 'xitcoind-symbols'
    run('build', ['go', 'build', '-mod=mod', '-o', str(binary), MODULE + '/cmd/evmd'], HARNESS)
    actual = run('binary-modules', ['go', 'list', '-m', '-json', 'all'], HARNESS)
    if graph(baseline) != graph(actual):
        raise RuntimeError('Versioned build changed the dependency graph')
    run('verify-sums', ['go', 'mod', 'verify'], HARNESS)
    buildinfo = run('buildinfo', ['go', 'version', '-m', str(binary)], HARNESS)
    if '(devel)' in buildinfo or resolved['Version'] not in buildinfo:
        raise RuntimeError('Binary is not bound to the published module version')
    packages = run('production-packages', ['go', 'list', '-deps', MODULE + '/cmd/evmd'], HARNESS).splitlines()
    forbidden = ('golang.org/x/crypto/openpgp', 'github.com/pion/dtls/v2',
                 'github.com/pion/stun/v2', 'cosmossdk.io/x/crisis',
                 'github.com/cosmos/cosmos-sdk/x/crisis',
                 'github.com/cosmos/cosmos-sdk/contrib/x/crisis')
    if any(p == prefix or p.startswith(prefix + '/') for p in packages for prefix in forbidden):
        raise RuntimeError('Previously removed production dependency is reachable')
    run('install-scanner', ['go', 'install', 'golang.org/x/vuln/cmd/govulncheck@v1.7.0'])
    gopath = subprocess.check_output(['go', 'env', 'GOPATH'], cwd=ROOT, text=True).strip()
    raw = run('binary-scan', [str(pathlib.Path(gopath) / 'bin/govulncheck'),
                              '-mode=binary', '-json', str(binary)], HARNESS)
    findings = [obj['finding'] for obj in objects(raw) if 'finding' in obj]
    for name, digest in original_locks.items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != digest:
            raise RuntimeError('Source lock changed during diagnostics: ' + name)
    summary = {'status': 'TECHNICAL_EVIDENCE_COLLECTED_REVIEW_REQUIRED',
               'security_acceptance': False, 'deployment': False, 'source_head': head,
               'published_version': resolved['Version'], 'dependency_graph_unchanged': True,
               'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
               'advisory_ids': sorted({f['osv'] for f in findings}), 'findings': findings,
               'scanner_exit_zero_does_not_mean_clean': True}
    (OUT / 'summary.json').write_text(json.dumps(summary, indent=2))


try:
    collect()
finally:
    # The binary is never executed or uploaded; only diagnostic evidence leaves the runner.
    binary_path = HARNESS / 'xitcoind-symbols'
    if binary_path.exists():
        binary_path.unlink()
