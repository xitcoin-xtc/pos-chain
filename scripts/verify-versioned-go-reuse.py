#!/usr/bin/env python3
"""Content-bound reuse of reviewed PR44 qualification; never security acceptance.

The policy and this verifier are reviewed repository code, not an independent
attestation. Changing both requires a new review and targeted qualification.
Only this verifier's digest field is omitted from the canonical policy digest
(to avoid a circular hash); its actual bytes are checked against that field.
"""
import copy
import hashlib
import json
from pathlib import Path
import stat
import subprocess
import zipfile

POLICY = 'docs/versioned-go-reuse.json'
VERIFIER = 'scripts/verify-versioned-go-reuse.py'
POLICY_SHA256 = 'd554ce71cbc6b2885f21625242fd4cae89961362dcb43a988cf29582610cb807'
QUALIFIED_HEAD = '5e24531951873ec8d0cf4403d118c45f47fa641e'
REVIEWED_HEAD = 'ca6af2f3a3ecfad3590709df23afbcbc450d3f82'
GATE_FILES = {'scripts/run-govulncheck.sh', 'scripts/test-govulncheck-gate.py',
              'docs/go-fork-provenance.md'}

MISSION_FILES = {'scripts/qualify-versioned-go.py', 'SECURITY-ASSESSMENT.md', 'AGENTS.md', 'scripts/verify-security-assessment.py', 'scripts/test-versioned-go-reuse.py', 'docs/development.md', 'scripts/run-govulncheck.sh', 'scripts/test-govulncheck-gate.py', 'README.md', 'UPSTREAMS.md', 'SECURITY.md', 'scripts/verify-versioned-go-reuse.py', 'docs/go-fork-provenance.md', '.github/workflows/versioned-go-qualification.yml', 'docs/security-assessment.json', 'scripts/verify-go-fork-provenance.py'}


def require(condition, message):
    # These additional checks must also fail with PYTHONOPTIMIZE=1.
    if not condition:
        raise ValueError(message)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical_policy(policy):
    value = copy.deepcopy(policy)
    value['mission_files'][VERIFIER]['sha256'] = 'SELF'
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()


def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args])


def tree(root, revision):
    entries = {}
    for entry in git(root, 'ls-tree', '-rz', revision).split(b'\0'):
        if entry:
            meta, path = entry.split(b'\t', 1)
            mode, kind, oid = meta.decode().split()
            entries[path.decode()] = (mode, kind, oid)
    return entries


def read_policy(root):
    raw = (root / POLICY).read_text()
    policy = json.loads(raw)
    require(raw == json.dumps(policy, indent=2) + '\n', 'reuse policy serialization changed')
    require(digest(canonical_policy(policy)) == POLICY_SHA256, 'reuse policy changed')
    require(policy['qualified_head'] == QUALIFIED_HEAD and
            policy['reviewed_head'] == REVIEWED_HEAD, 'inapplicable source heads')
    return policy


def validate_sources(root, policy):
    qualified = tree(root, QUALIFIED_HEAD)
    reviewed = tree(root, REVIEWED_HEAD)
    gate_delta = {p for p in qualified.keys() | reviewed.keys()
                  if qualified.get(p) != reviewed.get(p)}
    require(gate_delta == GATE_FILES, 'unreviewed gate delta')
    expected = dict(reviewed)
    mission = policy['mission_files']
    require(set(mission) == MISSION_FILES, 'unreviewed mission file set')
    for path in mission.keys() | {POLICY}:
        expected[path] = None
    current = tree(root, 'HEAD')
    require(set(current) <= set(expected), 'source beyond qualified scope changed')
    require(set(current) - set(mission) - {POLICY} == set(reviewed) - set(mission),
            'committed source deleted')
    staged = set(git(root, 'diff', '--cached', '--name-only', '-z').decode().split('\0')) - {''}
    require(staged <= set(mission) | {POLICY}, 'unqualified staged source')
    for path in staged:
        entry = git(root, 'ls-files', '--stage', '--', path).decode().strip().split()
        require(len(entry) == 4 and entry[2] == '0', 'missing/conflicted index entry')
        raw = git(root, 'show', ':' + path)
        expected_mode = '100644' if path == POLICY else mission[path]['mode']
        expected_digest = digest((root / POLICY).read_bytes()) if path == POLICY else mission[path]['sha256']
        require(entry[0] == expected_mode and digest(raw) == expected_digest,
                'unqualified staged mission content: ' + path)
    # Compare all committed entries, including modes and submodule pointers.
    for path, entry in current.items():
        if path not in mission and path != POLICY:
            require(entry == expected[path], 'committed source changed: ' + path)
    tracked = set(git(root, 'ls-files', '-z').decode().split('\0')) - {''}
    require(tracked <= set(expected), 'unqualified indexed source')
    for path, entry in expected.items():
        full = root / path
        if entry and entry[1] == 'commit':
            require(current.get(path) == entry, 'submodule changed: ' + path)
            require(subprocess.run(['git', '-C', str(root), 'diff', '--quiet',
                                    '--ignore-submodules=none', '--', path]).returncode == 0,
                    'submodule worktree changed: ' + path)
            continue
        require(full.is_file() and not full.is_symlink(), 'missing/nonregular file: ' + path)
        mode = '100755' if full.stat().st_mode & stat.S_IXUSR else '100644'
        if path == POLICY:
            require(mode == '100644', 'policy mode changed')
        elif path in mission:
            require(mode == mission[path]['mode'] and
                    digest(full.read_bytes()) == mission[path]['sha256'],
                    'mission content changed: ' + path)
        else:
            require(mode == entry[0] and hashlib.sha1(b'blob ' + str(full.stat().st_size).encode() + b'\0' + full.read_bytes()).hexdigest() == entry[2],
                    'source/lock/fixture changed: ' + path)
    # No directory exemption: only these exact generated output filenames may
    # be untracked. Even a Go file under versioned-evidence must be rejected.
    output_names = {'results.json', 'fetch-reviewed.log', 'fetch-baseline.log',
                    'preserved-checks.json', 'reused-checks.json', 'reuse-regressions.log',
                    'gate-regressions.log', 'qualification.json'}
    outputs = {'versioned-evidence/' + n for n in output_names}
    outputs |= {'versioned-evidence/historical-artifacts/' + a['file'] for a in policy['artifacts']}
    extras = set(git(root, 'ls-files', '--others', '-z').decode().split('\0')) - {''}
    require(extras - set(expected) <= outputs, 'unqualified untracked input')
    return {'qualified_head': QUALIFIED_HEAD, 'reviewed_head': REVIEWED_HEAD,
            'current_head': git(root, 'rev-parse', 'HEAD').decode().strip(),
            'unchanged_entries': len(reviewed) - len(mission.keys() & reviewed.keys()),
            'gate_files': sorted(GATE_FILES), 'mission_files': sorted(mission),
            'candidate_files_sha256': {p: digest((root / p).read_bytes()) for p in sorted(mission.keys() | {POLICY})},
            'worktree_differs_from_head': bool(git(root, 'diff', 'HEAD', '--name-only').strip())
                or not mission.keys() | {POLICY} <= current.keys()}


def download_history(policy, directory):
    directory.mkdir(parents=True, exist_ok=True)
    for item in policy['artifacts']:
        target = directory / item['file']
        if not target.exists():
            partial = target.with_suffix('.partial')
            with partial.open('wb') as output:
                subprocess.run(['gh', 'api', 'repos/xitcoin-xtc/pos-chain/actions/artifacts/'
                                + str(item['id']) + '/zip'], stdout=output, check=True)
            require(digest(partial.read_bytes()) == item['sha256'], 'download digest mismatch')
            partial.rename(target)


def validate_history(root, policy, directory):
    manifest = json.loads((root / 'docs/versioned-go-qualification-resume.json').read_text())
    preserved = {}
    for item in policy['artifacts']:
        path = directory / item['file']
        require(path.is_file(), 'historical artifact absent: ' + item['file'])
        require(digest(path.read_bytes()) == item['sha256'], 'historical artifact altered: ' + item['file'])
        initial = item['run_id'] == manifest['baseline_run']
        require(item['head_sha'] == (manifest['baseline_head'] if initial else QUALIFIED_HEAD),
                'historical artifact not applicable')
        with zipfile.ZipFile(path) as archive:
            names = archive.namelist()
            require(len(names) == len(set(names)) and all('/' not in n and n not in ('.', '..') for n in names),
                    'unexpected archive entries')
            results = json.loads(archive.read('results.json'))
            expected_results = item['result_labels']
            require(set(results) == set(expected_results) and bool(results), 'missing/unexpected historical results')
            for label, result in results.items():
                expected_exit = 1 if initial and item['module'] == 'evmd' and label == 'tests' else 0
                require(result['exit_code'] == expected_exit and result['blocked'] is None,
                        'historical result not reusable: ' + label)
                require(label + '.log' in names, 'historical result log absent: ' + label)
            q = json.loads(archive.read('qualification.json'))
            require(q['security_acceptance'] is False and q['review_expired'] == '2026-09-05'
                    and q['deployment'] is False, 'historical security disposition changed')
            require(q['status'] == ('QUALIFICATION_FAILED' if initial and item['module'] == 'evmd'
                                   else 'TECHNICAL_CHECKS_COMPLETED_REVIEW_REQUIRED'), 'historical status changed')
            assessment = json.loads(archive.read('production-graph-assessment.json'))
            require(assessment == {'forbidden_production_packages': [], 'forbidden_raw_key_rpc_files': []},
                    'historical production graph not reusable')
            if initial:
                require(results == manifest['prior_results'][item['module']], 'manifest results differ from artifact')
                if item['module'] == 'evmd':
                    require(digest(archive.read('tests.log')) == manifest['prior_evmd_tests_log_sha256'],
                            'initial failed tests log changed')
            else:
                require(json.loads(archive.read('preserved-checks.json')) == manifest,
                        'continuation manifest differs')
                if item['module'] == 'evmd':
                    require(results['tests']['command'] == ['go', 'test', '-tags=test', '-run',
                            '^Test(LedgerTestSuite|ERC20KeeperTestSuite)$', './tests/integration'],
                            'continuation does not cover failed suites')
                    info = archive.read('buildinfo.log').decode()
                    require('(devel)' not in info and 'v0.0.0-20260914101818-5e2453195187' in info,
                            'historical binary not applicable')
            preserved[item['file']] = {'run_id': item['run_id'], 'head_sha': item['head_sha'],
                                      'sha256': item['sha256'], 'module': item['module'],
                                      'results': results, 'qualification': q,
                                      'scanner_findings': json.loads(archive.read('scanner-findings.json'))}
    require(len(preserved) == 4, 'incomplete historical qualification')
    return preserved


def validate(root, directory):
    root = Path(root)
    policy = read_policy(root)
    sources = validate_sources(root, policy)
    history = validate_history(root, policy, Path(directory))
    return {'sources': sources, 'history': history, 'qualified_head': QUALIFIED_HEAD,
            'security_acceptance': False, 'historical_review_expired': '2026-09-05',
            'scans_are_historical': True, 'current_binary_built': False}
