import { accountWithPrefix } from '../common.js';
import {expect} from 'chai';

import { createTestRuntime } from '../../../../scripts/solidity-suite-runtime.mjs';
const hre = await createTestRuntime();

import { findEvent, waitWithTimeout, RETRY_DELAY_FUNC} from '../common.js';


describe('Distribution – set withdraw address', function () {
    const DIST_ADDRESS = '0x0000000000000000000000000000000000000801';
    const GAS_LIMIT = 1_000_000;

    let distribution, signer;

    before(async () => {
        [signer] = await hre.ethers.getSigners();
        distribution = await hre.ethers.getContractAt('DistributionI', DIST_ADDRESS);
    });

    it('should set withdraw address and emit SetWithdrawerAddress event', async function () {
        const newWithdrawAddress = accountWithPrefix('cosmos1fx944mzagwdhx0wz7k9tfztc8g3lkfk6pzezqh');
        const tx = await distribution
            .connect(signer)
            .setWithdrawAddress(signer.address, newWithdrawAddress, {gasLimit: GAS_LIMIT});
        const receipt = await waitWithTimeout(tx, 20000, RETRY_DELAY_FUNC);
        console.log('SetWithdrawAddress tx hash:', receipt.hash);

        const evt = findEvent(receipt.logs, distribution.interface, 'SetWithdrawerAddress');
        expect(evt, 'SetWithdrawerAddress event must be emitted').to.exist;
        expect(evt.args.caller).to.equal(signer.address);
        expect(evt.args.withdrawerAddress).to.equal(newWithdrawAddress);

        const withdrawer = await distribution.delegatorWithdrawAddress(signer.address);
        console.log('Withdraw address:', withdrawer);
        expect(withdrawer).to.equal(newWithdrawAddress);
    });
});