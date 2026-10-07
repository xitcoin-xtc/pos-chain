#!/usr/bin/env python3
"""Validate the exact current technical disposition; no independent approval."""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ASSESSMENT_SHA256 = '8ff999af550884af3fc54753793be44314ea205a093fd098565ac3059f28cc91'


def verify(root=ROOT):
    raw = (root / 'docs/security-assessment.json').read_bytes()
    if hashlib.sha256(raw).hexdigest() != ASSESSMENT_SHA256:
        raise ValueError('current security assessment changed; renewed review required')
    record = json.loads(raw)
    bindings = dict(record['lock_sha256'])
    bindings['SECURITY-ASSESSMENT.md'] = record['assessment_sha256']
    bindings['docs/go-fork-provenance.json'] = record['provenance_sha256']
    for name, expected in bindings.items():
        path = root / name
        if not path.is_file() or path.is_symlink() or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            raise ValueError('assessment input changed: ' + name)
    return record


if __name__ == '__main__':
    assessment = verify()
    print(assessment['assessed_on'], assessment['review_by'])
