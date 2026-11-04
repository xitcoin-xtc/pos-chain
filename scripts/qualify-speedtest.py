#!/usr/bin/env python3
"""Qualify the separate speedtest command on a disposable Linux runner."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

ROOT=Path(__file__).resolve().parents[1]
CWD=ROOT/'tests/speedtest'
OUT=ROOT/'speedtest-evidence'
OUT.mkdir(exist_ok=True)
RESULTS={}

def run(label,args,cwd=CWD):
    with (OUT/(label+'.log')).open('w') as output:
        result=subprocess.run(args,cwd=cwd,stdout=output,stderr=subprocess.STDOUT,timeout=2700)
    RESULTS[label]={'command':args,'exit_code':result.returncode}
    (OUT/'results.json').write_text(json.dumps(RESULTS,indent=2))
    print(label,result.returncode,flush=True)
    if result.returncode:
        print((OUT/(label+'.log')).read_text()[-5000:],flush=True)
        raise RuntimeError(label+' failed')
    return (OUT/(label+'.log')).read_text()

head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
assert head==os.environ['QUALIFICATION_HEAD_SHA']
record=json.loads((ROOT/'docs/security-assessment.json').read_text())
today=datetime.datetime.now(datetime.timezone.utc).date().isoformat()
assert record['assessed_on']<=today<=record['review_by']
run('assessment',['python3',str(ROOT/'scripts/verify-security-assessment.py')])
locks={name:hashlib.sha256((CWD/name).read_bytes()).hexdigest() for name in ['go.mod','go.sum']}
run('toolchain',['go','version'])
root=json.loads(run('root-replacements',['go','mod','edit','-json'],ROOT))
test=json.loads(run('speedtest-replacements',['go','mod','edit','-json']))
actual={item['Old']['Path']:item['New'] for item in test['Replace']}
for item in root['Replace']:
    assert actual[item['Old']['Path']]==item['New'],'speedtest replacement differs from reviewed root'
deps=run('effective-imports',['go','list','-deps','./...'])
assert not re.search(r'(?m)^(github.com/pion/(stun|dtls)/v2|golang.org/x/crypto/openpgp)(/|$)',deps)
run('modules',['go','list','-m','-json','all'])
run('module-verification',['go','mod','verify'])
run('install-scanner',['go','install','golang.org/x/vuln/cmd/govulncheck@v1.7.0'])
scanner=str(Path(subprocess.check_output(['go','env','GOPATH'],cwd=CWD,text=True).strip())/'bin/govulncheck')
run('strict-source-scan',[scanner,'./...'])
run('compile-and-tests',['go','test','-mod=readonly','-tags=test','-count=1','-timeout=30m','./...'])
binary=OUT/'speedtest'
run('build',['go','build','-mod=readonly','-trimpath','-o',str(binary),'./'])
run('binary-symbols',['go','tool','nm',str(binary)])
run('strict-binary-scan',[scanner,'-mode=binary',str(binary)])
# Cobra's wrapper may initialize a temporary app before displaying help;
# this runs only on the disposable runner, with no public RPC or keys.
run('command-help',[str(binary),'--help'])
assert locks=={name:hashlib.sha256((CWD/name).read_bytes()).hexdigest() for name in locks}
summary={'source_head':head,'status':'success','root_replacements_equal':True,
         'obsolete_effective_imports':False,'locks_unchanged':True,
         'scanner_exceptions':[],'historical_evidence_reused':False,
         'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),
         'deployment':False,'vpn_modified':False}
(OUT/'qualification.json').write_text(json.dumps(summary,indent=2))
# Keep only diagnostic evidence; do not publish an unversioned command binary.
binary.unlink()
