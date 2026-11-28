// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "./StakingSelfDestructor.sol";

/// @dev StakingSelfDestructorFactory creates a StakingSelfDestructor with
/// CREATE2, lets it stake, and destroys it in the same transaction, so that
/// EIP-6780 deletes its account (GHSA-c6xv-pppp-rjw6).
contract StakingSelfDestructorFactory {
    constructor() payable {}

    function predict(bytes32 salt) external view returns (address) {
        bytes32 hash = keccak256(
            abi.encodePacked(bytes1(0xff), address(this), salt, keccak256(type(StakingSelfDestructor).creationCode))
        );
        return address(uint160(uint256(hash)));
    }

    function createStakeAndDestroy(
        bytes32 salt,
        string calldata validator,
        uint256 amount,
        uint256 unbond
    ) external payable returns (address child) {
        StakingSelfDestructor created = new StakingSelfDestructor{salt: salt, value: msg.value}();
        created.stake(validator, amount, unbond);
        created.destroy(payable(msg.sender));
        child = address(created);
    }
}
