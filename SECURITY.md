# Security Policy

## Reporting a vulnerability

Report security vulnerabilities through GitHub Private Vulnerability Reporting for this repository. Include the affected component, impact, reproduction steps and any supporting evidence.

Do not open a public issue for an undisclosed vulnerability.

## Current Go dependency review

[Current review](docs/security-review-2026-11-04.md) records PR55's fresh
source scans, full tests, published-module binary evidence and residual risks.
The gate accepts no scanner advisory exceptions. It retains exact source,
manifest, embedded-resource, fork/checksum and dated-assessment checks;
obsolete production imports, raw-key RPC methods, scanner findings/errors,
source drift and expiry block CI.

The versioned workflow repeats qualification on the current checkout and exact
published PR head. It builds and tests the published modules, compares graphs,
verifies checksums and scans a symbol-bearing binary. No historical evidence
is reused to qualify current code. JSON output alone does not establish a clean
scan. The two module-only advisory matches remain visible with their traces.

[Historical assessment](SECURITY-ASSESSMENT.md) and
[its original record](docs/security-assessment-2026-09-14.json) retain all six
historical identities and their original dates. Immutable
[provenance](docs/go-fork-provenance.md) remains mandatory. The current review
is automated technical work, not independent approval or release authorization.
Renewal requires new justified evidence and tests, never just a date change.

## Scope

Reports may cover:

- consensus and state-transition logic;
- EVM execution and precompiles;
- staking, governance and validator admission;
- bridge authorization and accounting;
- build and release integrity;
- public RPC and API behavior implemented by this repository.

Third-party services and upstream dependencies should also be reported to their respective maintainers when appropriate.

## Disclosure

Please allow maintainers time to investigate and coordinate a fix before public disclosure. Receipt, assessment and remediation timelines depend on severity and reproducibility.
