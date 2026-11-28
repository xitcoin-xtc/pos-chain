# Transaction signatures

This page is for wallets, exchanges, explorers, relayers and other clients
that build or track Cosmos transactions on Xitcoin. It applies from the
`xitcoin-security-hardening-v2` upgrade (see the
[migration note](migrations/xitcoin-security-hardening-v2.md)). Ethereum
transactions sent through JSON-RPC (`MsgEthereumTx`) are not affected.

## Signature encoding of eth_secp256k1 keys

Xitcoin accounts use `/cosmos.evm.crypto.v1.ethsecp256k1.PubKey` keys. Every
signature of such a key in a Cosmos transaction, including each member
signature of a multisig, must be exactly 65 bytes `R || S || V`:

| What was signed | Produced by | `V` |
| --- | --- | --- |
| Keccak-256 of the sign bytes (`SIGN_MODE_DIRECT` `SignDoc`, or the `SIGN_MODE_LEGACY_AMINO_JSON` sign document) | `xitcoind` keyring (`tx sign`, `tx multisign`, every `tx` command), the Cosmos SDK keyring, `ethsecp256k1.PrivKey.Sign`, the bridge relayer | `0` or `1` |
| The EIP-712 typed data of the sign bytes | `eth_signTypedData_v4` (MetaMask), the Ledger Ethereum app (`xitcoind --ledger`) | `27` or `28` |

`S` must be in the lower half of the curve order, and `V` must be the
recovery ID of the signing key (plus 27 for an EIP-712 signature): of the two
candidate values only one is accepted. A transaction therefore has one valid
encoding and one transaction hash.

Rejected, with code `4` (`sdk` `ErrUnauthorized`):

- a 64-byte `R || S` signature: `signature 0: eth_secp256k1 signature must be 65 bytes (R || S || V), got 64`;
- a `V` other than 0, 1, 27 or 28: `eth_secp256k1 signature V must be 0 or 1 (27 or 28 for EIP-712 signatures), got …`;
- the other recovery ID, `V + 27` on a sign-bytes signature, `V - 27` on an EIP-712 signature, or a high `S`: `signature verification failed; …`.

A multisig member error names the member: `signature 0: multisig member 2: …`.
Keys of type `/cosmos.crypto.secp256k1.PubKey` keep the Cosmos SDK rule
(64 bytes, low `S`).

### Why

Before the upgrade the node dropped the 65th byte without looking at it and
also accepted 64 bytes. Anyone who saw a broadcast transaction could re-wrap
the same signed content with another `V` byte, or without it, and broadcast
it: a different `TxRaw` and transaction hash, valid at the same sequence and
paid by the original signer. Whichever copy a proposer included first
executed; the other failed with an account sequence mismatch. A client that
tracked its transaction by hash saw it fail although its transfer had
happened.

### What clients produce and what to change

- `xitcoind`, Go clients that sign with this chain's keyring or with
  go-ethereum's `crypto.Sign`, and the bridge relayer already produce the
  accepted form. No change.
- MetaMask and Ledger EIP-712 signatures: submit the 65 bytes that
  `eth_signTypedData_v4` returns, unchanged. A client that rewrote `V` to 0 or
  1 must add 27 back.
- Keplr returns a 64-byte `R || S` from `signDirect` and `signAmino` for
  eth_secp256k1 accounts, like other wallets that follow CosmJS conventions.
  When a wallet returns 64 bytes, append the recovery ID before encoding the
  `TxRaw`, then compute the transaction hash from the final bytes. With
  `@cosmjs/crypto`:

  ```ts
  import { ExtendedSecp256k1Signature, Secp256k1, keccak256 } from "@cosmjs/crypto";

  // signature: the 64 bytes returned by the wallet; signBytes: the bytes that
  // were signed (makeSignBytes(signDoc) for direct, serializeSignDoc(doc) for
  // amino); pubkey: the account's 33-byte compressed public key.
  function recoverableSignature(signature: Uint8Array, signBytes: Uint8Array, pubkey: Uint8Array): Uint8Array {
    const hash = keccak256(signBytes);
    for (const recovery of [0, 1]) {
      const candidate = new ExtendedSecp256k1Signature(signature.slice(0, 32), signature.slice(32, 64), recovery);
      const recovered = Secp256k1.compressPubkey(Secp256k1.recoverPubkey(candidate, hash));
      if (recovered.every((byte, i) => byte === pubkey[i])) {
        return candidate.toFixedLength(); // R || S || V, 65 bytes
      }
    }
    throw new Error("the signature does not belong to this account");
  }
  ```

  With ethers, `SigningKey.recoverPublicKey(keccak256(signBytes), { r, s, v: 27 + recovery })`
  does the same.
- Hermes signs eth_secp256k1 keys with a 64-byte signature and cannot be
  configured otherwise. An IBC relayer on Xitcoin must use a client that
  produces the accepted form or a `secp256k1` relayer key.
- Off-chain verifiers that call `ethsecp256k1.PubKey.VerifySignature` (for
  example for ADR-036 messages) get the same rule.

## Tracking transactions

- For `SIGN_MODE_DIRECT` transactions of single eth_secp256k1 signers the
  transaction hash is now a stable identifier.
- `SIGN_MODE_LEGACY_AMINO_JSON` (used by Ledger and by multisig) signs a
  JSON rendering of the transaction, not its protobuf bytes. The protobuf
  encoding of the body and auth info of such a transaction can still be
  rewritten without invalidating it ([Cosmos SDK ADR-076](https://github.com/cosmos/cosmos-sdk/blob/main/docs/architecture/adr-076-tx-malleability.md)).
  Track those transactions by signer and sequence (or by signature), not by
  hash alone, and treat an account sequence mismatch after a broadcast as
  "possibly included" until the sequence has been checked.

## Bridge transaction errors

A failed `x/bridge` transaction reports a registered code in codespace
`bridge`; see [bridge-architecture.md](bridge-architecture.md#error-codes).
