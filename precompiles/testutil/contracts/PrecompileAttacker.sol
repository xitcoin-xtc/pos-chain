// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "../../staking/StakingI.sol";
import "../../distribution/DistributionI.sol";
import "../../gov/IGov.sol";
import "../../ics20/ICS20I.sol";

/// @dev PrecompileAttacker drives the precompile attack simulations: reverts
/// after precompile calls, re-entrant call frames that mix precompile calls
/// and reverts, the per-transaction precompile call limit, and view calls
/// that must not change Cosmos state.
contract PrecompileAttacker {
    uint256 public completed;
    bool public lastDelegationSucceeded;

    constructor() payable {}

    receive() external payable {}

    /// @dev Delegates through the staking precompile, then reverts.
    function delegateThenRevert(string calldata validator, uint256 amount) public {
        STAKING_CONTRACT.delegate(address(this), validator, amount);
        revert("reverted after the precompile call");
    }

    /// @dev Sends native value, delegates, then reverts: the EVM balance
    /// change and the delegation must both be undone.
    function sendDelegateThenRevert(address payable to, uint256 value, string calldata validator, uint256 amount) public {
        to.transfer(value);
        STAKING_CONTRACT.delegate(address(this), validator, amount);
        revert("reverted after a transfer and the precompile call");
    }

    /// @dev Catches both reverting frames, then delegates once for real.
    function delegateAfterCaughtReverts(address payable to, uint256 value, string calldata validator, uint256 amount) external {
        try this.delegateThenRevert(validator, amount) {} catch {}
        try this.sendDelegateThenRevert(to, value, validator, amount) {} catch {}
        STAKING_CONTRACT.delegate(address(this), validator, amount);
        completed += 1;
    }

    /// @dev Re-enters itself `depth` times. Every frame delegates `amount`
    /// and sends 1 wei to `to`; frame `d` reverts when bit `d` of
    /// `revertMask` is set, after its children ran.
    function reenter(uint256 depth, uint256 revertMask, address payable to, string calldata validator, uint256 amount) public {
        STAKING_CONTRACT.delegate(address(this), validator, amount);
        to.transfer(1);
        if (depth > 0) {
            try this.reenter(depth - 1, revertMask, to, validator, amount) {} catch {}
        }
        if ((revertMask >> depth) & 1 == 1) {
            revert("frame reverted");
        }
        completed += 1;
    }

    /// @dev Makes `attempts` precompile calls that cost almost no gas
    /// (malformed ICS20 calldata, reverted by the precompile), optionally
    /// each inside a call frame that reverts, then attempts one delegation.
    function attemptsThenDelegate(
        uint256 attempts,
        bool insideRevertedFrames,
        string calldata validator,
        uint256 amount
    ) external returns (bool succeeded) {
        for (uint256 i = 0; i < attempts; i++) {
            if (insideRevertedFrames) {
                try this.malformedCallThenRevert() {} catch {}
            } else {
                malformedCall();
            }
        }
        try STAKING_CONTRACT.delegate(address(this), validator, amount) returns (bool) {
            succeeded = true;
        } catch {
            succeeded = false;
        }
        lastDelegationSucceeded = succeeded;
    }

    function malformedCall() internal {
        (bool ok, ) = ICS20_PRECOMPILE_ADDRESS.call(hex"01");
        ok;
    }

    function malformedCallThenRevert() external {
        malformedCall();
        revert("attempt reverted");
    }

    /// @dev View functions: the compiler calls precompiles from them with
    /// STATICCALL.
    function tally(uint64 proposalId) public view returns (TallyResultData memory) {
        return GOV_CONTRACT.getTallyResult(proposalId);
    }

    function totalRewards() public view returns (DecCoin[] memory total) {
        (, total) = DISTRIBUTION_CONTRACT.delegationTotalRewards(address(this));
    }

    /// @dev A transaction that only reads through view functions.
    function readViews(uint64 proposalId) external {
        tally(proposalId);
        totalRewards();
        completed += 1;
    }
}
