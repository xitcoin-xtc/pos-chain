#!/usr/bin/env python3
"""Isolated wrapper simulations, not Go graph validation or vulnerability scans.

Run with python3 scripts/test-govulncheck-gate.py. Only go, date and govulncheck
are simulated; the production Bash wrapper and Python provenance checker run.
Real root/evmd graph validation is required separately with acquired modules.
"""
import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
SDK = "github.com/cosmos/cosmos-sdk"
GETH = "github.com/ethereum/go-ethereum"
LIBRARY = "github.com/xitcoin-xtc/pos-chain"

FAKE_TOOL = r'''#!/usr/bin/env python3
import json, os, pathlib, sys
name = pathlib.Path(sys.argv[0]).name
cfg = json.loads(pathlib.Path(os.environ['GATE_CASE']).read_text())
root = pathlib.Path(os.environ['GATE_ROOT'])
module = 'evmd' if pathlib.Path.cwd() == root / 'evmd' else 'root'
with open(os.environ['GATE_TRACE'], 'a') as out:
    out.write(json.dumps([name, module, sys.argv[1:], os.environ.get('GOWORK')]) + '\n')
if name == 'date':
    print(cfg['date'])
elif name == 'govulncheck':
    print(cfg['scanner_output'])
    sys.exit(cfg['scanner_status'])
else:
    args = sys.argv[1:]
    modules = cfg['modules'][module]
    if args == ['env', 'GOMOD']:
        print(root / ('evmd/go.mod' if module == 'evmd' else 'go.mod'))
    elif args == ['list', '-m', '-json', 'all']:
        for obj in modules.values():
            print(json.dumps(obj))
    elif args == ['list', '-deps', './...']:
        print('\n'.join(cfg['packages'][module]))
    elif args[:3] == ['list', '-m', '-f']:
        obj = modules.get(args[-1], {})
        if args[3] == '{{.Version}}':
            print(obj.get('Version', ''))
        elif args[3] == '{{.Sum}} {{.GoModSum}}':
            print(obj.get('Sum', '') + ' ' + obj.get('GoModSum', ''))
        else:
            obj = obj.get('Replace', {})
            print((obj.get('Path', '') + ' ' + obj.get('Version', '')).strip())
    else:
        sys.exit('unsupported fake go invocation: ' + repr(args))
'''


class GateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="gate-simulation-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for name in ["scripts", "docs", "rpc", "evmd", "bin"]:
            (self.root / name).mkdir()
        for name in ["scripts/run-govulncheck.sh", "scripts/verify-go-fork-provenance.py",
                     "docs/go-fork-provenance.json", "docs/security-assessment.json",
                     "SECURITY-ASSESSMENT.md", "scripts/verify-security-assessment.py"]:
            shutil.copyfile(ROOT / name, self.root / name)
        for name in ["go.mod", "go.sum", "evmd/go.mod", "evmd/go.sum"]:
            shutil.copyfile(ROOT / name, self.root / name)
        for name in ["go", "date", "govulncheck"]:
            tool = self.root / "bin" / name
            tool.write_text(FAKE_TOOL)
            tool.chmod(0o755)
        manifest = json.loads((ROOT / "docs/go-fork-provenance.json").read_text())
        modules = {}
        for path, version in {
            SDK: "v0.54.4", GETH: "v1.16.9",
            "github.com/cometbft/cometbft": "v0.39.4",
            "github.com/pion/dtls/v2": "v2.2.12", "golang.org/x/crypto": "v0.56.0",
            "github.com/pion/stun/v3": "v3.1.5", "github.com/pion/dtls/v3": "v3.1.4",
            "github.com/ProtonMail/go-crypto": "v1.4.1",
        }.items():
            modules[path] = {"Path": path, "Version": version}
        for path, fork in manifest["forks"].items():
            modules[path]["Replace"] = {
                "Path": fork["path"], "Version": fork["version"],
                "Sum": fork["sum"], "GoModSum": fork["go_mod_sum"],
            }
        self.cfg = {"modules": {"root": copy.deepcopy(modules), "evmd": copy.deepcopy(modules)},
                    "packages": {"root": [LIBRARY + "/crypto"], "evmd": [LIBRARY + "/evmd"]},
                    "date": "2026-09-14", "scanner_output": "simulated scanner output", "scanner_status": 0}
        self.cfg["modules"]["root"][LIBRARY] = {"Path": LIBRARY, "Main": True}
        self.cfg["modules"]["evmd"][LIBRARY] = {
            "Path": LIBRARY, "Version": manifest["root_library"]["version"],
            "Sum": "h1:XlNgJS1pkjUXWEfxbl+8+B+5KnmI/AOT2ggB9uy2low=",
            "GoModSum": "h1:7EjObEwFuYAQRSd1W1j7D4hZCLB1SYLFazZHImpSZCE=",
        }

    def run_gate(self, module, expected, message="", scanned=False):
        config = self.root / "case.json"
        trace = self.root / "trace.jsonl"
        config.write_text(json.dumps(self.cfg))
        trace.write_text("")
        env = dict(os.environ, PATH=str(self.root / "bin") + os.pathsep + os.environ["PATH"],
                   GATE_CASE=str(config), GATE_TRACE=str(trace), GATE_ROOT=str(self.root), GOWORK="injected.work")
        cwd = self.root / ("evmd" if module == "evmd" else ".")
        proc = subprocess.run(["bash", str(self.root / "scripts/run-govulncheck.sh"), "./..."],
                              cwd=cwd, env=env, text=True, capture_output=True, timeout=30)
        output = proc.stdout + proc.stderr
        self.assertEqual(proc.returncode, expected, output)
        self.assertIn(message, output)
        calls = [json.loads(line) for line in trace.read_text().splitlines()]
        self.assertEqual(any(call[0] == "govulncheck" for call in calls), scanned, output)
        for tool, actual_module, _, work in calls:
            if tool != "date":
                self.assertEqual(actual_module, module)
                self.assertEqual(work, "off")

    def test_valid_locks_scanner_and_expiry(self):
        for module in ["root", "evmd"]:
            with self.subTest(module=module):
                self.cfg["date"] = "2026-09-14"
                self.run_gate(module, 0, scanned=True)
                self.cfg["date"] = "2026-09-22"
                self.run_gate(module, 1, "expired exception review still blocks", scanned=True)

    def test_current_assessment_boundaries_and_integrity(self):
        for module in ["root", "evmd"]:
            for date in ["2026-09-14", "2026-09-21"]:
                self.cfg["date"] = date
                self.run_gate(module, 0, scanned=True)
            for date in ["2026-09-13", "invalid"]:
                self.cfg["date"] = date
                self.run_gate(module, 1, "not yet valid")
        self.cfg["date"] = "2026-09-14"
        for name in ["docs/security-assessment.json", "SECURITY-ASSESSMENT.md",
                     "docs/go-fork-provenance.json", "go.mod", "go.sum",
                     "evmd/go.mod", "evmd/go.sum"]:
            path = self.root / name
            raw = path.read_bytes()
            try:
                path.write_bytes(raw + b"\n")
                for module in ["root", "evmd"]:
                    self.run_gate(module, 1, "assessment")
            finally:
                path.write_bytes(raw)

    def test_each_fork_replacement_and_sum(self):
        for module in ["root", "evmd"]:
            for path in [SDK, GETH]:
                original = copy.deepcopy(self.cfg["modules"][module][path])
                for field in ["absent", "Path", "Version", "Sum", "GoModSum"]:
                    with self.subTest(module=module, path=path, field=field):
                        obj = self.cfg["modules"][module][path]
                        if field == "absent":
                            obj.pop("Replace")
                        else:
                            obj["Replace"][field] = "incorrect"
                        message = "checksums differ" if field in ["Sum", "GoModSum"] else "replacement for"
                        self.run_gate(module, 1, message)
                        # evmd-only drift must not be mistaken for root drift.
                        if module == "evmd":
                            self.run_gate("root", 0, scanned=True)
                        self.cfg["modules"][module][path] = copy.deepcopy(original)

    def test_required_versions(self):
        for module in ["root", "evmd"]:
            for path in list(self.cfg["modules"][module]):
                if path == LIBRARY:
                    continue
                with self.subTest(module=module, path=path):
                    obj = self.cfg["modules"][module][path]
                    old = obj["Version"]
                    obj["Version"] = "v0.0.0"
                    self.run_gate(module, 1, "exception invalidated")
                    obj["Version"] = old

    def test_evmd_anchor(self):
        original = copy.deepcopy(self.cfg["modules"]["evmd"][LIBRARY])
        for field in ["Version", "Replace", "Sum", "GoModSum"]:
            with self.subTest(field=field):
                obj = self.cfg["modules"]["evmd"][LIBRARY]
                obj[field] = {"Path": LIBRARY, "Version": "v0.0.0"} if field == "Replace" else "incorrect"
                self.run_gate("evmd", 1, "root library")
                self.run_gate("root", 0, scanned=True)
                self.cfg["modules"]["evmd"][LIBRARY] = copy.deepcopy(original)

    def test_obsolete_production_imports(self):
        for module in ["root", "evmd"]:
            for path in ["github.com/pion/dtls/v2", "github.com/pion/stun/v2",
                         "golang.org/x/crypto/openpgp/armor", SDK + "/x/crisis",
                         SDK + "/contrib/x/crisis/types", "cosmossdk.io/x/crisis"]:
                with self.subTest(module=module, path=path):
                    self.cfg["packages"][module].append(path)
                    self.run_gate(module, 1, "production")
                    self.cfg["packages"][module].pop()

    def test_scanner_failures_and_advisory_identity(self):
        for module in ["root", "evmd"]:
            for status, output, expected in [
                (3, "Vulnerability #1: GO-2025-3442", 0),
                (3, "Vulnerability #1: GO-2099-9999", 1),
                (3, "unparseable", 1), (2, "scanner failure", 2),
            ]:
                with self.subTest(module=module, status=status, output=output):
                    self.cfg.update(scanner_status=status, scanner_output=output, date="2026-09-14")
                    self.run_gate(module, expected, scanned=True)
                    self.cfg["date"] = "2026-09-22"
                    self.run_gate(module, 1, "expired exception review still blocks", scanned=True)

    def test_existing_source_guards(self):
        for file, content, message in [
            ("rpc/obsolete.go", "func ImportRawKey() {}", "Raw private-key import"),
            ("crisis.go", 'import "github.com/cosmos/cosmos-sdk/x/crisis"', "deprecated Cosmos"),
        ]:
            with self.subTest(file=file):
                path = self.root / file
                path.write_text(content)
                self.run_gate("root", 1, message)
                path.unlink()


if __name__ == "__main__":
    unittest.main(verbosity=2)
