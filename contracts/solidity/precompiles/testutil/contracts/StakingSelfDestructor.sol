// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "../../staking/StakingI.sol";

/// @dev StakingSelfDestructor holds staking positions through the staking
/// precompile and self-destructs on request (GHSA-c6xv-pppp-rjw6).
contract StakingSelfDestructor {
    constructor() payable {}

    receive() external payable {}

    function stake(string calldata validator, uint256 amount, uint256 unbond) external {
        if (amount > 0) {
            STAKING_CONTRACT.delegate(address(this), validator, amount);
        }
        if (unbond > 0) {
            STAKING_CONTRACT.undelegate(address(this), validator, unbond);
        }
    }

    function destroy(address payable beneficiary) external {
        selfdestruct(beneficiary);
    }
}
