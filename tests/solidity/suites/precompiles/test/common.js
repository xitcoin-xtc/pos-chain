// Common constants and helper utilities for precompile tests

const STAKING_PRECOMPILE_ADDRESS = '0x0000000000000000000000000000000000000800'
const BECH32_PRECOMPILE_ADDRESS = '0x0000000000000000000000000000000000000400'
const DISTRIBUTION_PRECOMPILE_ADDRESS = '0x0000000000000000000000000000000000000801'
const BANK_PRECOMPILE_ADDRESS = '0x0000000000000000000000000000000000000804'
const GOV_PRECOMPILE_ADDRESS = '0x0000000000000000000000000000000000000805'
const SLASHING_PRECOMPILE_ADDRESS = '0x0000000000000000000000000000000000000806'
const P256_PRECOMPILE_ADDRESS = '0x0000000000000000000000000000000000000100'
const WERC20_ADDRESS = '0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE'

// Public fixture addresses follow the disposable node, never stored private keys.
const bech32 = require('bech32')
const { getAddress } = require('ethers')
const GENESIS_VALIDATOR = process.env.TEST_VALIDATOR_ADDRESS || 'cosmosvaloper10jmp6sgh4cc6zt3e8gw05wavvejgr5pw4xyrql'
const decodedValidator = bech32.decode(GENESIS_VALIDATOR)
if (!decodedValidator.prefix.endsWith('valoper')) throw new Error('Invalid test validator prefix')
const accountPrefix = decodedValidator.prefix.slice(0, -'valoper'.length)
const GENESIS_VALIDATOR_HEX = getAddress('0x' + Buffer.from(bech32.fromWords(decodedValidator.words)).toString('hex'))
function accountAddress(hex) {
    return bech32.encode(accountPrefix, bech32.toWords(Buffer.from(getAddress(hex).slice(2), 'hex')))
}
function validatorAddress(hex) {
    return bech32.encode(accountPrefix + 'valoper', bech32.toWords(Buffer.from(getAddress(hex).slice(2), 'hex')))
}
function accountWithPrefix(address) {
    return bech32.encode(accountPrefix, bech32.decode(address).words)
}

// Default gas limits used across tests
const DEFAULT_GAS_LIMIT = 1_000_000
const LARGE_GAS_LIMIT = 10_000_000

const RETRY_DELAY_FUNC = (attempt) => 500 * Math.pow(2, attempt)


function waitWithTimeout(txn, timeoutMs, retryDelayFn = (attempt) => 1000) {
    return new Promise((resolve, reject) => {
        let settled = false
        let retryTimer
        const finish = (error, result) => {
            if (settled) return
            settled = true
            clearTimeout(deadlineTimer)
            clearTimeout(retryTimer)
            if (error) reject(error)
            else resolve(result)
        }
        const deadlineTimer = setTimeout(() => {
            finish(new Error(`Txn wait failed after timeout of ${timeoutMs}ms.`))
        }, timeoutMs)
        let attempt = 0
        function attemptCall() {
            if (settled) return
            Promise.resolve().then(() => txn.wait()).then(
                result => finish(null, result),
                error => {
                    if (settled) return
                    if (error?.code === 'CALL_EXCEPTION' || error?.receipt?.status === 0 || error?.receipt?.status === 0n) {
                        finish(error)
                        return
                    }
                    retryTimer = setTimeout(attemptCall, retryDelayFn(++attempt))
                }
            )
        }
        attemptCall()
    })
}

// Helper to convert the raw tuple returned by staking.validator() into an object
function parseValidator(raw) {
    return {
        operatorAddress: raw[0],
        consensusPubkey: raw[1],
        jailed: raw[2],
        status: raw[3],
        tokens: raw[4],
        delegatorShares: raw[5],
        description: {
            moniker: raw[6][0],
            identity: raw[6][1],
            website: raw[6][2],
            securityContact: raw[6][3],
            details: raw[6][4],
        },
        unbondingHeight: raw[7],
        unbondingTime: raw[8],
        commission: raw[9],
        minSelfDelegation: raw[10]
    }
}

// Utility to parse logs and return the first matching event by name
function findEvent(logs, iface, eventName) {
    for (const log of logs) {
        try {
            const parsed = iface.parseLog(log)
            if (parsed && parsed.name === eventName) {
                return parsed
            }
        } catch {
            // ignore logs that do not match the contract interface
        }
    }
    return null
}

module.exports = {
    GENESIS_VALIDATOR,
    GENESIS_VALIDATOR_HEX,
    accountAddress,
    validatorAddress,
    accountWithPrefix,
    STAKING_PRECOMPILE_ADDRESS,
    BECH32_PRECOMPILE_ADDRESS,
    DISTRIBUTION_PRECOMPILE_ADDRESS,
    BANK_PRECOMPILE_ADDRESS,
    GOV_PRECOMPILE_ADDRESS,
    SLASHING_PRECOMPILE_ADDRESS,
    P256_PRECOMPILE_ADDRESS,
    WERC20_ADDRESS,
    DEFAULT_GAS_LIMIT,
    LARGE_GAS_LIMIT,
    RETRY_DELAY_FUNC,
    parseValidator,
    findEvent,
    waitWithTimeout
}
