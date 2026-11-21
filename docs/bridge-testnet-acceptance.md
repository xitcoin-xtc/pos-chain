# Xitcoin Bridge Testnet Acceptance

## Environment

Acceptance testing uses an isolated route and test assets. Production token governance and production bridge credentials are outside the test environment.

The public `xitcoin-testnet-v2-1` route remains disabled. Run the matrix on an isolated chain initialized with an approved test route. Operators submit inbound approvals with `xitcoind tx bridge submit-attestation` and users initiate outbound burns with `xitcoind tx bridge initiate-outbound`; neither command configures or enables a route. `xitcoind query bridge route` shows the route configuration, paused state and outstanding amount. Approvals must be produced with the v2 network-specific domains described in [bridge-architecture.md](bridge-architecture.md).

## Required tests

1. A finalized Cronos lock mints the exact native test XTC amount.
2. A finalized native XTC burn unlocks the exact Cronos test-token amount.
3. Minting without verified collateral is rejected.
4. Unlocking without a finalized burn is rejected.
5. A source deposit cannot be minted twice, including through a second approval that differs only in its deadline.
6. A modified chain, route, transaction, destination, amount or nonce is rejected.
7. An approval signed for another Xitcoin network, or carrying a source chain ID other than the route's, is rejected.
8. A signer below the configured threshold cannot authorize settlement.
9. The configured signer threshold authorizes a valid settlement.
10. An unknown or retired signer is rejected.
11. Pausing blocks new settlement and preserves accounting.
12. The guardian cannot mint, burn, release collateral or resume settlement.
13. Disabling the route (`update_route_config` with `enabled = false`) blocks new mints and burns; releases of burns that already happened remain possible on Cronos.
14. A control action whose nonce is not the next sequential nonce is rejected, so an older signed configuration cannot be applied after a newer one.
15. Per-transfer, daily and outstanding-mint limits are enforced.
16. Confirmation depth and source-chain reorganizations are handled correctly.
17. An expired approval is re-issued with a new deadline for the same deposit and mints exactly once.
18. A reconciliation deficit raises a critical alert and a guardian pause request; the pause itself is a separate guardian-signed action on each chain.
19. Native XTC minted minus native XTC burned equals represented collateral.
20. An outbound burn to the zero or dead address is rejected before burning, and a dead-address transfer cannot satisfy a burn requirement.

## Evidence

Each test record includes:

- test identifier and software revision;
- source event and destination transaction;
- route, addresses and amount;
- signer approvals;
- supply and collateral before and after;
- expected and observed results;
- genesis hash.

## Exit criteria

The testnet phase is complete when all required tests pass, authorization and replay protection are independently reviewed, accounting reconciles without mismatch, route limits and pause controls are validated, and operational procedures are approved.
