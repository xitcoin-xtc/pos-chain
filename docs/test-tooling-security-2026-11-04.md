# Current client toolchain migration review

The remaining JavaScript alerts cannot be fixed by overrides alone while
retaining retired Hardhat2 and Web3v1 dependency trees. This migration replaces
those test-only generations with Hardhat3.14 and Web3v4.16, preserves all case
names, Solidity fixtures and the protected governance opt-in policy, and
qualifies the recent APIs explicitly. It supersedes the earlier instruction
to retain original client generations during the initial Go/override review;
it does not claim continuing certification of Hardhat2 or Web3v1.

All23 Hardhat compatibility scenarios, seven Web3 scenarios and84 Solidity
cases must pass on disposable GitHub runners. The12 existing protected legacy
governance cases remain pending under their original explicit policy. No new
skips or dropped assertions are accepted. Compiler versions and optimization
settings are retained; moved Solidity source bytes are identical. No live
node, production application source, deployed contract, signing key, wallet,
VPN or migration is modified by this toolchain migration.

Hardhat uses the existing fixed Zod4.6.6 with its checksum-bound compatibility
adapter. Web3's validator also uses fixed Zod4.6.6; a checksum/version-bound
adapter translates the old refinement-metadata callback into superRefine while
retaining rejected fields, original values and format messages. Both shipped
CJS/ESM entry points are checked. Real regressions validate bounded-array early
rejection, ABI encoding, invalid arguments, offline signatures and CLI parser
errors. Every dependency audit must pass with zero vulnerabilities, without
advisory dismissals, aliases, ignored severities or accepted exceptions.

Root/evmd Go inputs and their exact reviewed assessment remain unchanged.
Speedtest's separate Go graph alignment is reviewed independently and requires
fresh Go qualification. This is a technical review, not independent security
approval or a claim that no other vulnerability can exist.
