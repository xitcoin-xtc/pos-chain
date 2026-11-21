#!/usr/bin/env python3
import hashlib,json,os,pathlib,re,shutil,signal,subprocess,sys,time
sys.dont_write_bytecode=True
if sys.flags.optimize: raise RuntimeError('qualification assertions require unoptimized Python')
root=pathlib.Path.cwd();module=sys.argv[1];cwd=root if module=='root' else root/'evmd';out=root/'versioned-evidence';out.mkdir(exist_ok=True);results={}
def run(label,args):
 minimum=shutil.disk_usage(root).free;start=time.monotonic();reason=None
 with (out/(label+'.log')).open('w') as log:
  p=subprocess.Popen(args,cwd=cwd,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
  while p.poll() is None:
   minimum=min(minimum,shutil.disk_usage(root).free)
   mem=int(next(l.split()[1] for l in pathlib.Path('/proc/meminfo').read_text().splitlines() if l.startswith('MemAvailable:')))*1024
   if minimum<5.25*1024**3 or mem<768*1024**2 or time.monotonic()-start>5400:
    reason='resource guard or stage deadline';os.killpg(p.pid,signal.SIGTERM)
    try:p.wait(timeout=10)
    except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait()
    break
   time.sleep(.5)
 results[label]={'command':args,'exit_code':p.returncode,'blocked':reason,'minimum_free_bytes':minimum,'seconds':round(time.monotonic()-start,2)}
 (out/'results.json').write_text(json.dumps(results,indent=2));print(label,json.dumps(results[label]),flush=True)
 if p.returncode:print((out/(label+'.log')).read_text()[-5000:])
 return p.returncode==0 and reason is None
resume_path=root/'docs/versioned-go-qualification-resume.json'
resume=None
import importlib.util
spec=importlib.util.spec_from_file_location('reuse',root/'scripts/verify-versioned-go-reuse.py')
reuse_guard=importlib.util.module_from_spec(spec);spec.loader.exec_module(reuse_guard)
policy=reuse_guard.read_policy(root)
if not run('fetch-reviewed',['git','-C',str(root),'fetch','--depth=2','origin',reuse_guard.REVIEWED_HEAD]):sys.exit(1)
# Full-tree identity and exact mission digests precede the original assertions.
reuse_guard.validate_sources(root,policy)
history_dir=out/'historical-artifacts'
reuse_guard.download_history(policy,history_dir)
reuse=reuse_guard.validate(root,history_dir)
if resume_path.exists():
 resume=json.loads(resume_path.read_text());baseline=resume['baseline_head']
 if not run('fetch-baseline',['git','-C',str(root),'fetch','--depth=1','origin',baseline]):sys.exit(1)
 def previous(path):return subprocess.check_output(['git','show',baseline+':'+path],cwd=root)
 changed=set(subprocess.check_output(['git','diff','--name-only',baseline,reuse['qualified_head']],cwd=root,text=True).splitlines())
 assert changed<=set(resume['allowed_changes']), 'source beyond qualified scope changed'
 for path,hashes in resume['fixtures'].items():
  assert hashlib.sha256(previous(path)).hexdigest()==hashes['before']
  assert hashlib.sha256((root/path).read_bytes()).hexdigest()==hashes['after']
 for path in ['go.mod','go.sum']:assert previous(path)==(root/path).read_bytes(), 'root locks changed'
 def normalize_evmd_mod(raw):return re.sub(rb'(?m)^(\s*github.com/xitcoin-xtc/pos-chain )\S+',rb'\1ROOT_VERSION',raw)
 assert normalize_evmd_mod(previous('evmd/go.mod'))==normalize_evmd_mod((root/'evmd/go.mod').read_bytes())
 def normalize_evmd_sum(raw):return b''.join(line for line in raw.splitlines(keepends=True) if not line.startswith(b'github.com/xitcoin-xtc/pos-chain '))
 assert normalize_evmd_sum(previous('evmd/go.sum'))==normalize_evmd_sum((root/'evmd/go.sum').read_bytes())
 before=json.loads(previous('docs/go-fork-provenance.json'));after=json.loads((root/'docs/go-fork-provenance.json').read_text())
 before.pop('root_library',None);after.pop('root_library',None);assert before==after
 assert all(v['exit_code']==0 and v['blocked'] is None for v in resume['prior_results']['root'].values())
 assert all(v['exit_code']==0 and v['blocked'] is None for k,v in resume['prior_results']['evmd'].items() if k!='tests')
 assert set(resume['failed_test_suites'])=={'TestLedgerTestSuite','TestERC20KeeperTestSuite'}
 assert resume['failed_test_packages']==['github.com/xitcoin-xtc/pos-chain/evmd/tests/integration']
 (out/'preserved-checks.json').write_text(json.dumps(resume,indent=2))
# The old fixture/lock/result assertions above remain mandatory. The new guard
# proves the current tree differs only by the reviewed, digest-bound gate delta.
if not resume: raise RuntimeError('historical qualification manifest absent')
(out/'reused-checks.json').write_text(json.dumps(reuse,indent=2))
if module=='root':
 run('reuse-regressions',['python3',str(root/'scripts/test-versioned-go-reuse.py')])
 run('gate-regressions',['python3',str(root/'scripts/test-govulncheck-gate.py')])
failed=any(v['exit_code']!=0 or v['blocked'] for v in results.values())
(out/'qualification.json').write_text(json.dumps({'status':'QUALIFICATION_FAILED' if failed else 'TECHNICAL_CHECKS_COMPLETED_REVIEW_REQUIRED','security_acceptance':False,'historical_review_expired':'2026-09-05','current_ci_assessment':'docs/security-assessment.json','deployment':False,'reuse_validated_for_head':reuse['sources']['current_head'],'historical_qualification_head':reuse['qualified_head'],'scans_are_historical':True,'current_binary_built':False},indent=2))
sys.exit(bool(failed))
# Historical non-reuse implementation retained for traceability, not executed.
run('modules',['go','list','-m','-json','all'])
run('provenance',['python3',str(root/'scripts/verify-go-fork-provenance.py')])
run('verify-sums',['go','mod','verify'])
if resume:
 if module=='root':run('fixture-build',['go','build','./tests/integration/wallets','./tests/integration/x/erc20'])
 else:run('tests',['go','test','-tags=test','-run','^Test(LedgerTestSuite|ERC20KeeperTestSuite)$','./tests/integration'])
else:
 run('build',['go','build','./...'])
 run('tests',['go','test','-tags=test','./...'])
run('production-graph',['go','list','-deps','./...'])
packages=set((out/'production-graph.log').read_text().splitlines())
forbidden=sorted(p for p in packages if p.startswith('golang.org/x/crypto/openpgp') or p.startswith('github.com/pion/dtls/v2') or p.startswith('github.com/cosmos/cosmos-sdk/x/crisis') or p.startswith('cosmossdk.io/x/crisis') or p.startswith('github.com/cosmos/cosmos-sdk/contrib/x/crisis'))
raw_key_files=[str(p.relative_to(root)) for p in (root/'rpc').rglob('*.go') if re.search(r'ImportRawKey|personal_importRawKey',p.read_text())]
(out/'production-graph-assessment.json').write_text(json.dumps({'forbidden_production_packages':forbidden,'forbidden_raw_key_rpc_files':raw_key_files},indent=2))
if run('install-scanner',['go','install','golang.org/x/vuln/cmd/govulncheck@v1.7.0']):
 scanner=str(pathlib.Path(subprocess.check_output(['go','env','GOPATH'],cwd=cwd,text=True).strip())/'bin/govulncheck')
 run('source-scan',[scanner,'-json','./...'])
 if module=='evmd':
  head=os.environ['QUALIFICATION_HEAD_SHA']
  resolved=run('resolve-published-evmd',['go','list','-m','-json','github.com/xitcoin-xtc/pos-chain/evmd@'+head])
  if resolved:
   resolved_text=(out/'resolve-published-evmd.log').read_text()
   version=json.JSONDecoder().raw_decode(resolved_text[resolved_text.index('{'):])[0]['Version']
   harness=root/'binary-qualification';harness.mkdir()
   shutil.copyfile(cwd/'go.mod',harness/'go.mod');shutil.copyfile(cwd/'go.sum',harness/'go.sum');cwd=harness
   run('prepare-versioned-binary',['go','mod','edit','-module=xitcoin.invalid/qualification','-require=github.com/xitcoin-xtc/pos-chain/evmd@'+version])
  else:version=None
 if module=='evmd' and version and run('build-symbols',['go','build','-mod=mod','-o',str(out/'xitcoind-symbols'),'github.com/xitcoin-xtc/pos-chain/evmd/cmd/evmd']):
  run('buildinfo',['go','version','-m',str(out/'xitcoind-symbols')])
  if '(devel)' in (out/'buildinfo.log').read_text():results['buildinfo']['exit_code']=1
  run('binary-symbols-scan',[scanner,'-mode=binary','-json',str(out/'xitcoind-symbols')])
  import hashlib
  (out/'xitcoind-symbols.sha256').write_text(hashlib.sha256((out/'xitcoind-symbols').read_bytes()).hexdigest()+'  xitcoind-symbols\n')
  # Keep the binary on the runner only; evidence contains build info, hash, and full scan.
  (out/'xitcoind-symbols').unlink()
# JSON scanner exit status alone is not a security disposition. Preserve every
# finding and its original trace, including modules hidden by fork coordinates.
scan_findings={}
for label in ['source-scan','binary-symbols-scan']:
 path=out/(label+'.log')
 if not path.exists(): continue
 raw=path.read_text(); findings=[]; decoder=json.JSONDecoder()
 try:
  while raw.strip():
   obj,end=decoder.raw_decode(raw.lstrip());raw=raw.lstrip()[end:]
   if 'finding' in obj: findings.append(obj['finding'])
 except json.JSONDecodeError as err:
  results[label]['parse_error']=str(err);results[label]['exit_code']=1
 scan_findings[label]={'ids':sorted({f['osv'] for f in findings}), 'findings':findings}
(out/'scanner-findings.json').write_text(json.dumps(scan_findings,indent=2))
failed=forbidden or raw_key_files or any(v['exit_code']!=0 or v['blocked'] for v in results.values())
(out/'results.json').write_text(json.dumps(results,indent=2))
(out/'qualification.json').write_text(json.dumps({'status':'QUALIFICATION_FAILED' if failed else 'TECHNICAL_CHECKS_COMPLETED_REVIEW_REQUIRED','security_acceptance':False,'review_expired':'2026-09-05','deployment':False},indent=2))
sys.exit(bool(failed))
