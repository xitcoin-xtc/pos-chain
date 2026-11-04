#!/usr/bin/env python3
"""Validate the exact current technical disposition; no independent approval."""
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[1]
ASSESSMENT_SHA256 = '545d3969c41a66134cf10e003b637c2b2e8d2998ac578facbb6a6ff546c90e5e'


def source_inputs(root):
    names = subprocess.check_output(
        ['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z'],
        cwd=root).decode().split('\0')
    sources = {name for name in names if name.endswith(('.go', '.proto'))
               or Path(name).name in ('go.mod', 'go.sum')}
    for name in list(sources):
        path = root / name
        if not path.is_file() or path.is_symlink() or not path.resolve().is_relative_to(root.resolve()):
            raise ValueError('invalid assessment source input: ' + name)
        if not name.endswith('.go'):
            continue
        for embedded in re.findall(r'(?m)^\s*//go:embed\s+([^\n]+)', (root / name).read_text()):
            for resource in embedded.split():
                if any(c in resource for c in '*?["'):
                    raise ValueError('embedded resource pattern requires a new review: ' + name)
                sources.add(str(Path(name).parent / resource))
    for name in sorted(sources):
        path = root / name
        if not path.is_file() or path.is_symlink() or not path.resolve().is_relative_to(root.resolve()):
            raise ValueError('invalid assessment source input: ' + name)
    return sorted(sources)


def source_digest(root):
    digest = hashlib.sha256()
    for name in source_inputs(root):
        digest.update(name.encode() + b'\0' + hashlib.sha256((root / name).read_bytes()).digest())
    return digest.hexdigest()


def verify(root=ROOT):
    raw = (root / 'docs/security-assessment.json').read_bytes()
    if hashlib.sha256(raw).hexdigest() != ASSESSMENT_SHA256:
        raise ValueError('current security assessment changed; renewed review required')
    record = json.loads(raw)
    if record.get('accepted_scanner_advisories') != []:
        raise ValueError('current assessment must not accept scanner exceptions')
    if source_digest(root) != record['go_source_sha256']:
        raise ValueError('assessment source inputs changed; renewed review required')
    bindings = dict(record['lock_sha256'])
    bindings[record['assessment_document']] = record['assessment_sha256']
    bindings['docs/go-fork-provenance.json'] = record['provenance_sha256']
    for name, expected in bindings.items():
        path = root / name
        if not path.is_file() or path.is_symlink() or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            raise ValueError('assessment input changed: ' + name)
    return record


if __name__ == '__main__':
    assessment = verify()
    print(assessment['assessed_on'], assessment['review_by'])
