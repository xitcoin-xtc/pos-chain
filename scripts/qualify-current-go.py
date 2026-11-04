#!/usr/bin/env python3
"""Fresh checkout and published-module qualification; no historical reuse."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
MODULE = sys.argv[1]
if MODULE not in ('root', 'evmd'):
    raise ValueError('Expected root or evmd')
CWD = ROOT if MODULE == 'root' else ROOT / 'evmd'
PACKAGE = 'github.com/xitcoin-org/pos-chain' + ('/evmd' if MODULE == 'evmd' else '')
OUT = ROOT / 'versioned-evidence'
OUT.mkdir(exist_ok=True)
HARNESS = ROOT / ('current-versioned-harness-' + MODULE)
RESULTS = {}
BINARY = HARNESS / 'xitcoind-symbols'


def run(label, args, cwd=CWD):
    started = time.monotonic()
    reason = None
    with (OUT / (label + '.log')).open('w') as stream:
        proc = subprocess.Popen(args, cwd=cwd, stdout=stream, stderr=subprocess.STDOUT,
                                start_new_session=True)
        while proc.poll() is None:
            free = shutil.disk_usage(ROOT).free
            available = next(int(line.split()[1]) * 1024 for line in
                             Path('/proc/meminfo').read_text().splitlines()
                             if line.startswith('MemAvailable:'))
            if free < 5.25 * 1024**3 or available < 768 * 1024**2 or time.monotonic() - started > 5400:
                reason = 'Resource guard or stage deadline'
                os.killpg(proc.pid, signal.SIGTERM)
                try:
                    proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    os.killpg(proc.pid, signal.SIGKILL)
                    proc.wait()
                break
            time.sleep(0.5)
    RESULTS[label] = {'command': args, 'exit_code': proc.returncode, 'blocked': reason,
                      'seconds': round(time.monotonic() - started, 2)}
    (OUT / 'results.json').write_text(json.dumps(RESULTS, indent=2))
    print(label, proc.returncode, reason or '', flush=True)
    if proc.returncode or reason:
        print((OUT / (label + '.log')).read_text()[-5000:], flush=True)
        raise RuntimeError('Qualification failed: ' + label)
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
    for item in objects(raw):
        if item.get('Main') or item['Path'] == PACKAGE:
            continue
        replacement = item.get('Replace', {})
        if not item.get('Version') or (replacement and not replacement.get('Version')):
            raise RuntimeError('Unversioned dependency: ' + item['Path'])
        result[item['Path']] = (item['Version'], replacement.get('Path'), replacement.get('Version'))
    return result


summary = {'status': 'QUALIFICATION_FAILED', 'security_acceptance': False,
           'deployment': False, 'historical_evidence_reused': False, 'module': MODULE}
try:
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    if head != os.environ['QUALIFICATION_HEAD_SHA']:
        raise RuntimeError('Checkout does not match the requested qualification head')
    summary['source_head'] = head
    original = {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest()
                for name in ('go.mod', 'go.sum', 'evmd/go.mod', 'evmd/go.sum')}
    run('toolchain', ['go', 'version'])
    run('assessment', ['python3', str(ROOT / 'scripts/verify-security-assessment.py')])
    if MODULE == 'root':
        run('gate-regressions', ['python3', str(ROOT / 'scripts/test-govulncheck-gate.py')])
    run('provenance', ['python3', str(ROOT / 'scripts/verify-go-fork-provenance.py')])
    baseline = run('checkout-modules', ['go', 'list', '-m', '-json', 'all'])
    run('checkout-sums', ['go', 'mod', 'verify'])
    run('install-scanner', ['go', 'install', 'golang.org/x/vuln/cmd/govulncheck@v1.7.0'])
    scanner = str(Path(subprocess.check_output(['go', 'env', 'GOPATH'], cwd=CWD, text=True).strip()) / 'bin/govulncheck')
    os.environ['PATH'] = str(Path(scanner).parent) + os.pathsep + os.environ['PATH']
    run('checkout-strict-scan', ['bash', str(ROOT / 'scripts/run-govulncheck.sh'), './...'])
    run('checkout-tests', ['go', 'test', '-mod=readonly', '-tags=test', '-count=1', '-timeout=60m', './...'])
    run('checkout-build', ['go', 'build', '-mod=readonly', './...'])
    resolved = json.loads(run('published-module', ['go', 'list', '-m', '-json', PACKAGE + '@' + head]))
    if resolved.get('Origin', {}).get('Hash') != head:
        raise RuntimeError('Published module does not resolve to the reviewed commit')
    HARNESS.mkdir()
    for name in ('go.mod', 'go.sum'):
        shutil.copyfile(CWD / name, HARNESS / name)
    run('prepare-published', ['go', 'mod', 'edit', '-module=xitcoin.invalid/current-qualification',
                             '-require=' + PACKAGE + '@' + resolved['Version']], HARNESS)
    target = PACKAGE + '/cmd/evmd' if MODULE == 'evmd' else PACKAGE + '/...'
    build = ['go', 'build', '-mod=mod']
    if MODULE == 'evmd':
        build += ['-o', str(BINARY)]
    run('published-build', build + [target], HARNESS)
    run('published-tests', ['go', 'test', '-mod=mod', '-tags=test', '-count=1', '-timeout=60m', PACKAGE + '/...'], HARNESS)
    actual = run('published-modules', ['go', 'list', '-m', '-json', 'all'], HARNESS)
    if graph(baseline) != graph(actual):
        raise RuntimeError('Published dependency graph differs from the checkout')
    run('published-sums', ['go', 'mod', 'verify'], HARNESS)
    run('published-strict-scan', [scanner, PACKAGE + '/...'], HARNESS)
    raw = run('published-json-scan', [scanner, '-json', PACKAGE + '/...'], HARNESS)
    summary['source_findings'] = [o['finding'] for o in objects(raw) if 'finding' in o]
    if MODULE == 'evmd':
        buildinfo = run('binary-buildinfo', ['go', 'version', '-m', str(BINARY)], HARNESS)
        if '(devel)' in buildinfo or resolved['Version'] not in buildinfo:
            raise RuntimeError('Binary is not bound to the published version')
        run('binary-strict-scan', [scanner, '-mode=binary', str(BINARY)], HARNESS)
        raw = run('binary-json-scan', [scanner, '-mode=binary', '-json', str(BINARY)], HARNESS)
        summary['binary_findings'] = [o['finding'] for o in objects(raw) if 'finding' in o]
        summary['binary_sha256'] = hashlib.sha256(BINARY.read_bytes()).hexdigest()
    for name, digest in original.items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != digest:
            raise RuntimeError('Source lock changed: ' + name)
    summary.update(status='FRESH_TECHNICAL_QUALIFICATION_PASSED', strict_scans_passed=True,
                   dependency_graph_unchanged=True, published_version=resolved['Version'], locks=original)
finally:
    if BINARY.exists():
        BINARY.unlink()
    (OUT / 'qualification.json').write_text(json.dumps(summary, indent=2))
