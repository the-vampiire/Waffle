import {providers} from 'ethers';
import {toUtf8String} from 'ethers/lib/utils';
import {Provider} from 'ganache';
import {log} from './log';

/* eslint-disable no-control-regex */

/**
 * Decodes a revert string from a failed call/query that reverts on chain.
 * @param callRevertError The error catched from performing a reverting call (query)
 */
export const decodeRevertString = (callRevertError: any): string => {
  /**
   * https://ethereum.stackexchange.com/a/66173
   * Numeric.toHexString(Hash.sha3("Error(string)".getBytes())).substring(0, 10)
   */
  const errorMethodId = '0x08c379a0';

  if (!callRevertError.data?.startsWith(errorMethodId)) return '';
  return toUtf8String('0x' + callRevertError.data.substring(138))
    .replace(/\x00/g, ''); // Trim null characters.
};

const appendRevertString = async (etherProvider: providers.Web3Provider, receipt: any) => {
  if (parseInt(receipt.status) === 0) {
    log('Got transaction receipt of a failed transaction. Attempting to replay to obtain revert string.');
    try {
      const tx = await etherProvider.getTransaction(receipt.transactionHash);
      log('Running transaction as a call:');
      log(tx);
      // Run the transaction as a query. It works differently in Ethers, a revert code is included.
      await etherProvider.call(tx as any, tx.blockNumber);
    } catch (error: any) {
      log('Caught error, attempting to extract revert string from:');
      log(error);
      receipt.revertString = decodeRevertString(error);
      log(`Extracted revert string: "${receipt.revertString}"`);
    }
  }
};

/**
 * Ethers executes a gas estimation before sending the transaction to the blockchain.
 * This poses a problem for Waffle - we cannot track sent transactions which eventually revert.
 * This is a common use case for testing, but such transaction never gets sent.
 * A failed gas estimation prevents it from being sent.
 *
 * In test environment, we replace the gas estimation with an always-succeeding method.
 * If a transaction is meant to be reverted, it will do so after it is actually send and mined.
 *
 * Additionally, we patch the method for getting transaction receipt.
 * Ethers does not provide the error code in the receipt that we can use to
 * read a revert string, so we patch it and include it using a query to the blockchain.
 */
export const injectRevertString = (provider: Provider): Provider => {
  const etherProvider = new providers.Web3Provider(provider as any);
  return new Proxy(provider, {
    get(target, prop, receiver) {
      const original = (target as any)[prop as any];
      if (typeof original !== 'function') {
        // Some non-method property - returned as-is.
        return original;
      }
      // Return a function override.
      return function (...args: any[]) {
        // Get a function result from the original provider.
        const originalResult = original.apply(target, args);

        // Every method other than `provider.request()` left intact.
        if (prop !== 'request') return originalResult;

        const method = args[0]?.method;
        /**
         * A method can be:
         * - `eth_estimateGas` - gas estimation, typically precedes `eth_sendRawTransaction`.
         * - `eth_getTransactionReceipt` - getting receipt of sent transaction,
         *    typically supersedes `eth_sendRawTransaction`.
         * Other methods left intact.
         */
        if (method === 'eth_estimateGas') {
          return (async () => {
            try {
              return await originalResult;
            } catch (e) {
              return '0xE4E1C0'; // 15_000_000
            }
          })();
        } else if (method === 'eth_sendRawTransaction') {
          /**
           * Because we have overriden the gas estimation not to be failing on reverts,
           * we add a wait during transaction sending to retain original behaviour of
           * having an exception when sending a failing transaction.
           */
          return (async () => {
            const transactionHash = await originalResult;
            const tx = await etherProvider.getTransaction(transactionHash);
            try {
              await tx.wait(); // Will end in an exception if the transaction is failing.
            } catch (e: any) {
              log('Transaction failed after sending and waiting.');
              await appendRevertString(etherProvider, e.receipt);
              throw e;
            }
            return transactionHash;
          })();
        } else if (method === 'eth_getTransactionReceipt') {
          return (async () => {
            const receipt = await originalResult;
            await appendRevertString(etherProvider, receipt);
            return receipt;
          })();
        }
        return originalResult; // Fallback for any other method.
      };
    }
  });
};
