# Validator admission attack simulations

The validator admission policy ([validator-admission.md](validator-admission.md))
is attacked by a Go native fuzz target on the ante handler and by a bypass
table on the real `evmd` application. `go test -tags=test ./x/validatoradmission/...`
replays the fuzz seed corpus and `go test -tags=test ./tests/integration/validatoradmission/`
(in `evmd`) runs the bypass table, so each known attack stays a regression
test. The `Attack-simulation fuzzing` workflow (`.github/workflows/fuzz.yml`)
explores the fuzz target further every night with a time box and uploads any
failing input as an artifact; it commits nothing.

## Fuzz target

| Target | Property |
| --- | --- |
| `x/validatoradmission.FuzzAdmissionAnteHandler` | The input is decoded into an environment (approved operators, on-chain minimum including an unusable one, executable governance on or off, genesis or later height) and a tree of up to four messages: authz `MsgExec` up to ten levels deep, gov v1 proposals carrying messages, gov v1beta1 proposals, `MsgCreateValidator` (approved, unapproved, upper-case or malformed operator; missing, zero, below, at and above the minimum value and `min_self_delegation`; wrong denomination), `MsgUnjail`, mint `MsgUpdateParams`, `MsgGrant` for every restricted type URL and for others (generic, stake, send, nil and unknown authorizations), unknown `Any` types, `MsgEditValidator`, `MsgRevoke`, `MsgApproveValidator` and `MsgExecLegacyContent`. A reference walker written from the policy decides the outcome: a restricted message (validator creation or unjail, mint parameters, and proposal submission while governance is disabled) inside a `MsgExec` or a proposal, a grant for one, an unresolvable `Any`, nesting at depth 7 or more, an unapproved operator, a value or (after genesis) `min_self_delegation` below the minimum, or any mint parameter change is rejected. The handler never panics, never lets a transaction the reference rejects reach the next ante handler, always accepts a transaction the reference accepts, and decides the same after the messages are encoded and decoded through the interface registry. |

## Bypass table

Each restricted action is attempted through every path that reaches the
message servers, on the real application (`evmd/tests/integration/validatoradmission`).
Every attempt must leave the protected state unchanged; a positive control
shows that each path that can succeed for an approved operator really reaches
the message server.

| Path | Create a validator without approval | `min_self_delegation` below the minimum (creation, edit) | Unjail a revoked validator | Re-create a revoked validator | Change mint parameters | Submit a proposal (governance disabled) |
| --- | --- | --- | --- | --- | --- | --- |
| Direct transaction | rejected (ante) | rejected (ante; edit: x/staking) | rejected (ante) | rejected (ante) | rejected (ante) | rejected (ante) |
| Self-granted `MsgExec` | rejected | edit rejected (x/staking) | rejected | rejected | rejected | rejected |
| `MsgExec` nested 2 to 10 deep | rejected | | rejected | | | rejected |
| Grant, then `MsgExec` by the grantee | grant and exec rejected | | grant and exec rejected | | grant rejected | grant rejected |
| Precompile from the EOA | rejected (wrapper, hook) | creation rejected (wrapper) | rejected (wrapper) | rejected | n/a | rejected (submission disabled) |
| Precompile from a contract (CALL) | rejected (contracts cannot create) | | rejected (caller is not the operator) | | n/a | rejected |
| DELEGATECALL, CALLCODE | rejected (read-only) | | rejected (read-only) | | n/a | rejected (read-only) |
| EIP-7702 delegated account | rejected (wrapper), control passes once approved | creation rejected (wrapper), edit rejected (x/staking); controls pass at and above the minimum | rejected, control passes while approved | rejected | n/a | rejected |
| ICS20 source callbacks (ack, timeout) and destination callback | rejected; callbacks only call contracts, control delegation passes | | rejected | | n/a | rejected |
| Governance proposal (governance enabled) | rejected at submission | | rejected at submission | | rejected at submission (also through `MsgExec`, a grant, a nested proposal and the governance precompile) | n/a |
| Revocation spelled in upper case | | | rejected | | | |

Last lines of defence, tested directly on the application:

- the admission staking hook rejects an unapproved operator and a
  `min_self_delegation` below the minimum when the plain `x/staking` message
  server (the handler that the router, authz and `x/gov` dispatch to) is
  called without the ante handler or the precompile wrapper, and when the
  hook itself is called;
- the admission governance hook rejects a proposal carrying mint parameters
  when the plain `x/gov` message server is called, and rejects every proposal
  while executable governance is disabled;
- no interchain-accounts host or controller and no `x/group` are wired: no
  module, store, message handler, registered type or IBC port.

## Findings (fixed for `xitcoin-security-hardening-v2`)

| Property | On `main` | Now |
| --- | --- | --- |
| The ante handler never panics | **failed**: a `MsgCreateValidator` without `value.amount` (a nil integer, as decoded from a transaction that omits it) panicked for an approved operator | holds |
| A governance proposal cannot carry a restricted message (governance enabled) | **failed**: the ante handler did not look inside proposals and the governance precompile skips it; a proposal changing the mint inflation bounds passed and executed | holds (ante handler and `x/gov` hook) |
| A revocation removes the approval it names | **failed**: a revocation spelled in upper case jailed the validator but left the lower-case approval, so it could unjail at once | holds (approvals are canonical; the upgrade removes non-canonical entries) |
| Every other cell of the bypass table | held | holds |

See [the migration note](migrations/xitcoin-security-hardening-v2.md#validator-admission-changes)
for the consensus impact.

## Running locally

```bash
go test -tags=test ./x/validatoradmission/...                          # seed corpus
(cd evmd && go test -tags=test ./tests/integration/validatoradmission/) # bypass table
go test -tags=test -run='^$' -fuzz='^FuzzAdmissionAnteHandler$' \
  -fuzztime=5m -fuzzminimizetime=10s ./x/validatoradmission/
```

A failing input is written to `x/validatoradmission/testdata/fuzz/FuzzAdmissionAnteHandler/`.
Reproduce it with `go test -tags=test -run='FuzzAdmissionAnteHandler/<file>'`,
fix the code (never the property or the reference), and add the input to the
seeds.
