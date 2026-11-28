import { expect } from 'chai';
import { bech32 } from 'bech32';
import { getAddress } from 'ethers';

import { createTestRuntime } from '../../../../scripts/solidity-suite-runtime.mjs';
const hre = await createTestRuntime();


describe('Slashing – query methods', function () {
    const SLASHING_ADDRESS = '0x0000000000000000000000000000000000000806';
    // Consensus address of the genesis validator of the local node
    // (`evmd comet show-address`), exported by load_test_validator_cons_address.
    const consBech32 = process.env.TEST_VALIDATOR_CONS_ADDRESS
    if (!consBech32) throw new Error('TEST_VALIDATOR_CONS_ADDRESS is not set')
    const CONS_ADDR = getAddress('0x' + Buffer.from(bech32.fromWords(bech32.decode(consBech32).words)).toString('hex'))
    let slashing;

    before(async function () {
        slashing = await hre.ethers.getContractAt('ISlashing', SLASHING_ADDRESS);
    });

    it('getSigningInfos returns list of signing info', async function () {
        const pageReq = { key: '0x', offset: 0, limit: 10, countTotal: true, reverse: false };
        const [infos, pageResponse] = await slashing.getSigningInfos(pageReq);
        console.log('Signing infos:', infos, 'Page:', pageResponse);
        expect(infos.length).to.be.greaterThan(0);
        expect(pageResponse.total).to.be.a('bigint').and.to.be.greaterThan(0n);
        // Another validator may be listed first (the staking tests create one).
        const info = infos.find((i) => i.validatorAddress === CONS_ADDR);
        expect(info, 'signing info of the genesis validator').to.exist;
        expect(info.startHeight).to.be.a('bigint');
        expect(info.indexOffset).to.be.a('bigint');
        expect(info.tombstoned).to.be.a('boolean');
        expect(info.missedBlocksCounter).to.be.a('bigint');
    });

    it('getSigningInfo returns info for a validator', async function () {
        const info = await slashing.getSigningInfo(CONS_ADDR);
        console.log('Signing info:', info);
        expect(info.validatorAddress).to.equal(CONS_ADDR);
        expect(info.startHeight).to.be.a('bigint');
    });

    it('getParams returns slashing module params', async function () {
        const params = await slashing.getParams();
        console.log('Params:', params);
        expect(params.signedBlocksWindow).to.be.a('bigint');
        expect(params.minSignedPerWindow.value).to.be.a('bigint');
        expect(params.downtimeJailDuration).to.be.a('bigint');
        expect(params.slashFractionDoubleSign.value).to.be.a('bigint');
        expect(params.slashFractionDowntime.value).to.be.a('bigint');
    });
});
