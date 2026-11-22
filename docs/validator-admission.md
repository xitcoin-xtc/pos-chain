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
their genesis transactions.

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

## Revocation

Revocation removes the operator from the approval set. An existing validator is jailed and cannot unjail until it is approved again, whether it sends `MsgUnjail` directly, through `MsgExec` or through the slashing precompile.

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
