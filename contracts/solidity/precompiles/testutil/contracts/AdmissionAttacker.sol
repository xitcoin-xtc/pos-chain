// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity ^0.8.0;

/// @dev AdmissionAttacker forwards prepared calldata (for example a staking,
/// slashing or governance precompile call) with CALL, DELEGATECALL or
/// CALLCODE: immediately, from the IBC source callbacks, or as the delegated
/// code of an EIP-7702 account. A failed forward never reverts the caller, so
/// that a test observes the chain state the attempt left behind.
contract AdmissionAttacker {
    uint8 public constant MODE_CALL = 0;
    uint8 public constant MODE_DELEGATECALL = 1;
    uint8 public constant MODE_CALLCODE = 2;

    address public armedTarget;
    bytes public armedData;
    uint8 public armedMode;

    event Forwarded(address indexed target, uint8 mode, bool success, bytes result);

    constructor() payable {}

    receive() external payable {}

    /// @dev arm stores the call that forwardArmed and the IBC callbacks make.
    function arm(address target, bytes calldata data, uint8 mode) external {
        armedTarget = target;
        armedData = data;
        armedMode = mode;
    }

    function forward(
        address target,
        bytes calldata data,
        uint8 mode
    ) external payable returns (bool success, bytes memory result) {
        return _forward(target, data, mode);
    }

    function forwardArmed() public returns (bool success, bytes memory result) {
        return _forward(armedTarget, armedData, armedMode);
    }

    function onPacketAcknowledgement(
        string calldata,
        string calldata,
        uint64,
        bytes calldata,
        bytes calldata
    ) external {
        forwardArmed();
    }

    function onPacketTimeout(
        string calldata,
        string calldata,
        uint64,
        bytes calldata
    ) external {
        forwardArmed();
    }

    function _forward(
        address target,
        bytes memory data,
        uint8 mode
    ) internal returns (bool success, bytes memory result) {
        if (mode == MODE_CALL) {
            (success, result) = target.call(data);
        } else if (mode == MODE_DELEGATECALL) {
            (success, result) = target.delegatecall(data);
        } else if (mode == MODE_CALLCODE) {
            assembly ("memory-safe") {
                success := callcode(gas(), target, 0, add(data, 0x20), mload(data), 0, 0)
                let size := returndatasize()
                result := mload(0x40)
                mstore(result, size)
                returndatacopy(add(result, 0x20), 0, size)
                mstore(0x40, add(add(result, 0x20), and(add(size, 0x1f), not(0x1f))))
            }
        } else {
            revert("unknown mode");
        }
        emit Forwarded(target, mode, success, result);
    }
}
