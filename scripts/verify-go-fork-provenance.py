#!/usr/bin/env python3
"""Validate exact fork locks; advisory mapping is retained independently of scanners."""
import json,pathlib,subprocess,sys
root=pathlib.Path(__file__).resolve().parents[1]
manifest=json.loads((root/'docs/go-fork-provenance.json').read_text())
raw=subprocess.check_output(['go','list','-m','-json','all'],text=True)
decoder=json.JSONDecoder();modules={}
while raw.strip():
 obj,end=decoder.raw_decode(raw.lstrip());raw=raw.lstrip()[end:];modules[obj['Path']]=obj
errors=[]
for name,obj in modules.items():
 replacement=obj.get('Replace')
 if replacement and not replacement.get('Version'):errors.append(f'{name}: local or unversioned replacement')
 if not obj.get('Main') and not obj.get('Version'):errors.append(f'{name}: missing module version')
for name,expected in manifest['forks'].items():
 actual=modules.get(name,{});replacement=actual.get('Replace',{})
 if len(expected['commit'])!=40 or not expected['version'].endswith('-'+expected['commit'][:12]):errors.append(f'{name}: version does not pin the recorded commit')
 if actual.get('Version')!=expected['required_version']:errors.append(f'{name}: original required version changed')
 if (replacement.get('Path'),replacement.get('Version'))!=(expected['path'],expected['version']):errors.append(f'{name}: immutable fork replacement mismatch')
 if not replacement.get('Sum') or not replacement.get('GoModSum'):errors.append(f'{name}: missing Go-generated checksums')
 if (replacement.get('Sum'),replacement.get('GoModSum'))!=(expected.get('sum'),expected.get('go_mod_sum')):errors.append(f'{name}: checksums differ from published provenance')
library=modules.get('github.com/xitcoin-xtc/pos-chain')
if library and not library.get('Main'):
 expected_root=manifest.get('root_library',{})
 if library.get('Version')!=expected_root.get('version') or library.get('Replace'):errors.append('evmd: root library is not pinned to the recorded published revision')
 if not expected_root.get('commit') or not library.get('Version','').endswith('-'+expected_root['commit'][:12]):errors.append('evmd: root library commit/version mismatch')
packages=set(subprocess.check_output(['go','list','-deps','./...'],text=True).splitlines())
residual_modules=[]
for forbidden in ['github.com/pion/dtls/v2','github.com/pion/stun/v2']:
 if forbidden in modules:residual_modules.append({'path':forbidden,'version':modules[forbidden].get('Version'),'review_required':True})
 if any(p==forbidden or p.startswith(forbidden+'/') for p in packages):errors.append(f'{forbidden}: obsolete production dependency reintroduced')
ids={a['id'] for a in manifest['advisories']}
if ids!={'GO-2023-1821','GO-2023-1881','GO-2024-2584','GO-2025-3442','GO-2026-4479','GO-2026-5932'}:errors.append('six-advisory provenance set changed')
# Historical identity manifest: current CI dates live in security-assessment.json.
if manifest['review_expires']!='2026-09-05':errors.append('historical review expiry changed')
report={'status':'FAIL' if errors else 'LOCKS_VERIFIED_REVIEW_STILL_REQUIRED','errors':errors,'residual_modules_requiring_review':residual_modules,'advisories':manifest['advisories'],'forks':manifest['forks']}
print(json.dumps(report,indent=2));sys.exit(bool(errors))
