# Validator Admission

Xitcoin uses an on-chain validator admission policy.

## Policy

| Parameter | Initial value |
| --- | ---: |
| Maximum approved validator operators | 258 |
| Minimum self-delegation | 5,000,000 XTC |

Amounts are encoded in `axtc`, with 18 decimal places.

## Admission flow

1. An operator submits its validator operator address for review.
2. The configured authority approves the operator address on-chain.
3. The operator signs `MsgCreateValidator` directly, with a self-delegation
   (`value`) of at least the minimum and `min_self_delegation` also at least the
   minimum (in `axtc`).
4. The ante handler verifies approval and the active policy.
5. The staking module applies the standard consensus and staking checks and the
   validator-admission staking hook checks approval and `min_self_delegation`
   again.

Because `min_self_delegation` is at least the minimum, the staking module itself
keeps enforcing the minimum after creation: a validator whose self-delegation
falls below it is jailed. Genesis validators keep the `min_self_delegation` of
their genesis transactions until the `xitcoin-security-hardening-v2` upgrade,
which raises it to the minimum for every validator whose self-delegation covers
the minimum and reports the others without changing them (see
[the migration note](migrations/xitcoin-security-hardening-v2.md)).

## Enforcement paths

- `MsgCreateValidator` and `MsgUnjail` must be signed directly by the operator.
  They are rejected inside an authz `MsgExec` at any nesting depth, including a
  `MsgExec` whose granter and grantee are the same account. A `MsgGrant` for these
  message types is rejected.
- Validators created through the staking precompile go through the same checks
  (message-server wrapper and staking hook). Unjailing through the slashing
  precompile checks approval as well. Both precompiles are inactive on the
  testnet.
- Mint parameter changes and governance proposals (gov v1 and v1beta1) are
  rejected at top level and inside `MsgExec`.
- Executable governance proposals are disabled in release binaries. Where they
  are enabled (the legacy integration suites), a proposal cannot carry a
  restricted message or a grant for one, at any depth: `x/gov` executes the
  messages of a passed proposal without the ante handler, so the ante handler
  checks them like the messages of a `MsgExec`, and an `x/gov` hook checks
  every new proposal whatever the submission path (including the governance
  precompile). The hook rejects every proposal while proposals are disabled.

## Revocation

Revocation removes the operator from the approval set. An existing validator is jailed and cannot unjail until it is approved again, whether it sends `MsgUnjail` directly, through `MsgExec` or through the slashing precompile.

Approvals are stored and looked up under the canonical (lower-case bech32) operator address, so an approval or a revocation acts on the operator it names whatever the spelling (the bech32 decoder also accepts the upper-case form).

## Policy updates

The configured authority may update the approval capacity and minimum self-delegation. The approval capacity cannot be reduced below the number of approved operators.

## Query and transaction commands

Discover available commands:

```bash
xitcoind query validator-admission --help
xitcoind tx validator-admission --help
```

Inspect transaction flags:

```bash
xitcoind tx validator-admission update-params --help
```

Always specify the intended chain ID, node endpoint and fee denomination when preparing a transaction.
