package seth

import (
	"bytes"
	"context"
	"encoding/hex"
	verr "errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/avast/retry-go"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const (
	ErrDecodeInput          = "failed to decode transaction input"
	ErrDecodeOutput         = "failed to decode transaction output"
	ErrDecodeLog            = "failed to decode log"
	ErrDecodedLogNonIndexed = "failed to decode non-indexed log data"
	ErrDecodeILogIndexed    = "failed to decode indexed log data"
	ErrTooShortTxData       = "tx data is less than 4 bytes, can't decode"
	ErrRPCJSONCastError     = "failed to cast CallMsg error as rpc.DataError"
	ErrUnableToDecode       = "unable to decode revert reason"

	WarnNoContractStore = "ContractStore is nil, use seth.NewContractStore(...) to decode transactions"
)

// DecodedTransaction decoded transaction
type DecodedTransaction struct {
	CommonData
	Index       uint                    `json:"index"`
	Hash        string                  `json:"hash,omitempty"`
	Protected   bool                    `json:"protected,omitempty"`
	Transaction *types.Transaction      `json:"transaction,omitempty"`
	Receipt     *types.Receipt          `json:"receipt,omitempty"`
	Events      []DecodedTransactionLog `json:"events,omitempty"`
}

type CommonData struct {
	CallType        string                 `json:"call_type,omitempty"`
	Signature       string                 `json:"signature"`
	Method          string                 `json:"method"`
	Input           map[string]interface{} `json:"input,omitempty"`
	Output          map[string]interface{} `json:"output,omitempty"`
	NestingLevel    int                    `json:"nesting_level,omitempty"`
	ParentSignature string                 `json:"parent_signature,omitempty"`
	Error           string                 `json:"error,omitempty"`
}

// DecodedCall decoded call
type DecodedCall struct {
	CommonData
	FromAddress string             `json:"from_address,omitempty"`
	ToAddress   string             `json:"to_address,omitempty"`
	From        string             `json:"from,omitempty"`
	To          string             `json:"to,omitempty"`
	Events      []DecodedCommonLog `json:"events,omitempty"`
	Comment     string             `json:"comment,omitempty"`
	Value       int64              `json:"value,omitempty"`
	GasLimit    uint64             `json:"gas_limit,omitempty"`
	GasUsed     uint64             `json:"gas_used,omitempty"`
}

type DecodedCommonLog struct {
	Signature string                 `json:"signature"`
	Address   common.Address         `json:"address"`
	EventData map[string]interface{} `json:"event_data"`
	Topics    []string               `json:"topics,omitempty"`
}

func getDefaultDecodedCall() *DecodedCall {
	return &DecodedCall{
		CommonData: CommonData{
			CallType:  UNKNOWN,
			Signature: UNKNOWN,
			Method:    UNKNOWN,
			Input:     make(map[string]interface{}),
			Output:    make(map[string]interface{}),
		},
		FromAddress: UNKNOWN,
		ToAddress:   UNKNOWN,
		From:        UNKNOWN,
		To:          UNKNOWN,
		Events:      make([]DecodedCommonLog, 0),
	}
}

// Decode waits for transaction to be minted, then decodes transaction inputs, outputs, logs and events and
// depending on 'tracing_level' it either returns immediately or if the level matches it traces all calls.
// Where tracing results are sent depends on the 'trace_outputs' field in the config.
// If transaction was reverted the error returned will be revert error, not decoding error (that, if any, will only be logged).
// At the same time we also return decoded transaction, so contrary to go convention you might get both error and result,
// because we want to return the decoded transaction even if it was reverted.
// Last, but not least, if gas bumps are enabled, we will try to bump gas on transaction mining timeout and resubmit it with higher gas.
func (m *Client) Decode(tx *types.Transaction, txErr error) (*DecodedTransaction, error) {
	if len(m.Errors) > 0 {
		return nil, verr.Join(m.Errors...)
	}

	if decodedErr := m.DecodeSendErr(txErr); decodedErr != nil {
		return nil, decodedErr
	}

	return m.DecodeTx(tx)
}

// DecodeSendErr tries to decode the error and return the reason of the revert. If the error is not revert, it returns the original error.
// If the error is revert, but it cannot be decoded, it logs the error and returns the original error.
// If the error is revert, and it can be decoded, it returns the decoded error.
// This function is used to decode errors that are returned by the send transaction function.
func (m *Client) DecodeSendErr(txErr error) error {
	if txErr == nil {
		return nil
	}

	reason, decodingErr := m.DecodeCustomABIErr(txErr)

	if decodingErr == nil && reason != "" {
		return errors.Wrap(txErr, reason)
	}

	L.Trace().
		Msg("No decode-able error found, returning original error")
	return txErr
}

// DecodeTx waits for transaction to be minted, then decodes transaction inputs, outputs, logs and events and
// depending on 'tracing_level' and transaction status (reverted or not) it either returns immediately or traces all calls.
// If transaction was reverted the error returned will be revert error, not decoding error (that, if any, will only be logged).
// At the same time we also return decoded transaction, so contrary to go convention you might get both error and result,
// because we want to return the decoded transaction even if it was reverted.
// Last, but not least, if gas bumps are enabled, we will try to bump gas on transaction mining timeout and resubmit it with higher gas.
func (m *Client) DecodeTx(tx *types.Transaction) (*DecodedTransaction, error) {
	if tx == nil {
		L.Trace().
			Msg("Skipping decoding, because transaction is nil. Nothing to decode")
		return nil, nil
	}

	l := L.With().Str("Transaction", tx.Hash().Hex()).Logger()

	if m.Cfg.Hooks != nil && m.Cfg.Hooks.Decode.Pre != nil {
		if err := m.Cfg.Hooks.Decode.Pre(m); err != nil {
			return nil, err
		}
	} else {
		l.Trace().
			Msg("No pre-decode hook found. Skipping")
	}

	var receipt *types.Receipt
	var err error
	tx, receipt, err = m.waitUntilMined(l, tx)
	if err != nil {
		return nil, err
	}

	var revertErr error
	if receipt.Status == 0 {
		revertErr = m.callAndGetRevertReason(tx, receipt)
	}

	decoded, decodeErr := m.decodeTransaction(l, tx, receipt)

	if m.Cfg.Hooks != nil && m.Cfg.Hooks.Decode.Post != nil {
		if err := m.Cfg.Hooks.Decode.Post(m, decoded, decodeErr); err != nil {
			return nil, err
		}
	} else {
		l.Trace().
			Msg("No post-decode hook found. Skipping")
	}

	if decodeErr != nil && errors.Is(decodeErr, errors.New(ErrNoABIMethod)) {
		m.handleTxDecodingError(l, *decoded, decodeErr)
		return decoded, revertErr
	}

	if m.Cfg.TracingLevel == TracingLevel_None {
		m.handleDisabledTracing(l, *decoded)
		return decoded, revertErr
	}

	if m.Cfg.TracingLevel == TracingLevel_All || (m.Cfg.TracingLevel == TracingLevel_Reverted && revertErr != nil) {
		decodedCalls, traceErr := m.Tracer.TraceGethTX(decoded.Hash)
		if traceErr != nil {
			m.handleTracingError(l, *decoded, traceErr, revertErr)
			return decoded, revertErr
		}

		m.handleSuccessfulTracing(l, *decoded, decodedCalls, revertErr)
	} else {
		l.Trace().
			Str("Tracing level", m.Cfg.TracingLevel).
			Bool("Was reverted?", revertErr != nil).
			Msg("Transaction doesn't match tracing level, skipping decoding")
	}

	return decoded, revertErr
}

func (m *Client) waitUntilMined(l zerolog.Logger, tx *types.Transaction) (*types.Transaction, *types.Receipt, error) {
	// if transaction was not mined, we will retry it with gas bumping, but only if gas bumping is enabled
	// and if the transaction was not mined in time, other errors will be returned as is
	var receipt *types.Receipt
	err := retry.Do(
		func() error {
			var err error
			ctx, cancel := context.WithTimeout(context.Background(), m.Cfg.Network.TxnTimeout.Duration())
			receipt, err = m.WaitMined(ctx, l, m.Client, tx)
			cancel()

			return err
		}, retry.OnRetry(func(i uint, retryErr error) {
			replacementTx, replacementErr := prepareReplacementTransaction(m, tx)
			if replacementErr != nil {
				L.Debug().Str("Replacement error", replacementErr.Error()).Str("Current error", retryErr.Error()).Uint("Attempt", i).Msg("Failed to prepare replacement transaction. Retrying with the original one")
				return
			}
			l.Debug().Str("Current error", retryErr.Error()).Uint("Attempt", i).Msg("Waiting for transaction to be confirmed after gas bump")
			tx = replacementTx
		}),
		retry.DelayType(retry.FixedDelay),
		// unless attempts is at least 1 retry.Do() won't execute at all
		retry.Attempts(func() uint {
			if m.Cfg.GasBumpRetries() == 0 {
				return 1
			}
			return m.Cfg.GasBumpRetries()
		}()),
		retry.RetryIf(func(err error) bool {
			return m.Cfg.GasBumpRetries() != 0 && errors.Is(err, context.DeadlineExceeded)
		}),
	)

	if err != nil {
		l.Trace().
			Err(err).
			Msg("Skipping decoding, because transaction was not mined. Nothing to decode")
		return nil, nil, err
	}

	return tx, receipt, nil
}

func (m *Client) handleTxDecodingError(l zerolog.Logger, decoded DecodedTransaction, decodeErr error) {
	tx := decoded.Transaction

	if m.Cfg.hasOutput(TraceOutput_JSON) {
		l.Trace().
			Err(decodeErr).
			Msg("Failed to decode transaction. Saving transaction data hash as JSON")

		err := CreateOrAppendToJsonArray(m.Cfg.revertedTransactionsFile, tx.Hash().Hex())
		if err != nil {
			l.Warn().
				Err(err).
				Str("TXHash", tx.Hash().Hex()).
				Msg("Failed to save reverted transaction hash to file")
		} else {
			l.Trace().
				Str("TXHash", tx.Hash().Hex()).
				Msg("Saved reverted transaction to file")
		}
	}

	if m.Cfg.hasOutput(TraceOutput_Console) {
		m.printDecodedTXData(l, &decoded)
	}
}

func (m *Client) handleTracingError(l zerolog.Logger, decoded DecodedTransaction, traceErr, revertErr error) {
	if m.Cfg.hasOutput(TraceOutput_JSON) {
		l.Trace().
			Err(traceErr).
			Msg("Failed to trace call, but decoding was successful. Saving decoded data as JSON")

		path, saveErr := saveAsJson(decoded, filepath.Join(m.Cfg.ArtifactsDir, "traces"), decoded.Hash)
		if saveErr != nil {
			l.Warn().
				Err(saveErr).
				Msg("Failed to save decoded call as JSON")
		} else {
			l.Trace().
				Str("Path", path).
				Str("Tx hash", decoded.Hash).
				Msg("Saved decoded transaction data to JSON")
		}
	}

	if strings.Contains(traceErr.Error(), "debug_traceTransaction does not exist") {
		l.Debug().
			Msg("Debug API is either disabled or not available on the node. Disabling tracing")

		l.Error().
			Err(revertErr).
			Msg("Transaction was reverted, but we couldn't trace it, because debug API on the node is disabled")

		m.Cfg.TracingLevel = TracingLevel_None
	}

	if m.Cfg.hasOutput(TraceOutput_Console) {
		m.printDecodedTXData(l, &decoded)
	}
}

func (m *Client) handleSuccessfulTracing(l zerolog.Logger, decoded DecodedTransaction, decodedCalls []*DecodedCall, revertErr error) {
	if m.Cfg.hasOutput(TraceOutput_JSON) {
		path, saveErr := saveAsJson(m.Tracer.GetDecodedCalls(decoded.Hash), filepath.Join(m.Cfg.ArtifactsDir, "traces"), decoded.Hash)
		if saveErr != nil {
			l.Warn().
				Err(saveErr).
				Msg("Failed to save decoded call as JSON")
		} else {
			l.Trace().
				Str("Path", path).
				Str("Tx hash", decoded.Hash).
				Msg("Saved decoded call data to JSON")
		}
	}

	if m.Cfg.hasOutput(TraceOutput_Console) {
		m.Tracer.printDecodedCallData(L, decodedCalls, revertErr)
		if err := m.Tracer.PrintTXTrace(decoded.Hash); err != nil {
			l.Trace().
				Err(err).
				Msg("Failed to print decoded call data")
		}
	}

	if m.Cfg.hasOutput(TraceOutput_DOT) {
		if err := m.Tracer.generateDotGraph(decoded.Hash, decodedCalls, revertErr); err != nil {
			l.Trace().
				Err(err).
				Msg("Failed to generate DOT graph")
		}
	}
}

func (m *Client) handleDisabledTracing(l zerolog.Logger, decoded DecodedTransaction) {
	tx := decoded.Transaction
	L.Trace().
		Str("Transaction Hash", tx.Hash().Hex()).
		Msg("Tracing level is NONE, skipping decoding")
	if m.Cfg.hasOutput(TraceOutput_Console) {
		m.printDecodedTXData(l, &decoded)
	}
}

// MergeEventData merges new event data into the existing EventData map in the DecodedTransactionLog.
func (d *DecodedCommonLog) MergeEventData(newEventData map[string]interface{}) {
	if d.EventData == nil {
		d.EventData = make(map[string]interface{})
	}
	for k, v := range newEventData {
		d.EventData[k] = v
	}
}

// DecodedTransactionLog decoded Solidity log(event)
type DecodedTransactionLog struct {
	DecodedCommonLog
	BlockNumber uint64 `json:"block_number"`
	Index       uint   `json:"index"`
	TXHash      string `json:"hash"`
	TXIndex     uint   `json:"tx_index"`
	Removed     bool   `json:"removed"`
	FileTag     string `json:"file_tag,omitempty"`
}

func (d *DecodedTransactionLog) MergeEventData(newEventData map[string]interface{}) {
	if d.EventData == nil {
		d.EventData = make(map[string]interface{})
	}
	for k, v := range newEventData {
		d.EventData[k] = v
	}
}

// decodeTransaction decodes inputs/outputs/logs of a transaction, if tx have logs with topics all topics are decoded
// if `tracing_enabled` flag is set in Client config it will also automatically trace all transaction calls and print the trace
func (m *Client) decodeTransaction(l zerolog.Logger, tx *types.Transaction, receipt *types.Receipt) (*DecodedTransaction, error) {
	var txInput map[string]interface{}
	var txEvents []DecodedTransactionLog
	txData := tx.Data()
	defaultTxn := &DecodedTransaction{
		Receipt:     receipt,
		Transaction: tx,
		Protected:   tx.Protected(),
		Hash:        tx.Hash().String(),
	}

	if len(txData) == 0 && tx.Value() != nil && tx.Value().Cmp(big.NewInt(0)) > 0 {
		l.Debug().Msg("Transaction has no data. It looks like a simple ETH transfer and there is nothing to decode")
		return defaultTxn, nil
	}

	// this might indicate a malformed tx, but we can't be sure, so we just log it and continue
	if len(txData) < 4 {
		l.Debug().Msgf("Transaction data is too short to decode. Expected at last 4 bytes, but got %d. Skipping decoding", len(txData))
		return defaultTxn, nil
	}
	if m.ContractStore == nil {
		l.Warn().Msg(WarnNoContractStore)
		return defaultTxn, nil
	}

	sig := txData[:4]
	if m.ABIFinder == nil {
		l.Err(errors.New("ABIFInder is nil")).Msg("ABIFinder is required for transaction decoding")
		return defaultTxn, nil
	}

	var address string
	if tx.To() != nil {
		address = tx.To().String()
	} else {
		address = UNKNOWN
	}

	abiResult, err := m.ABIFinder.FindABIByMethod(address, sig)
	if err != nil {
		return defaultTxn, err
	}

	txInput, err = decodeTxInputs(l, txData, abiResult.Method)
	if err != nil {
		return defaultTxn, errors.Wrap(err, ErrDecodeInput)
	}

	var txIndex uint

	if receipt != nil {
		l.Trace().Interface("Receipt", receipt).Msg("TX receipt")
		logsValues := make([]types.Log, 0)
		for _, l := range receipt.Logs {
			logsValues = append(logsValues, *l)
		}

		var allABIs []*abi.ABI
		if m.ContractStore == nil {
			allABIs = append(allABIs, &abiResult.ABI)
		} else {
			allABIs = m.ContractStore.GetAllABIs()
		}

		txEvents, err = m.decodeContractLogs(l, logsValues, allABIs)
		if err != nil {
			return defaultTxn, err
		}
		txIndex = receipt.TransactionIndex
	}
	ptx := &DecodedTransaction{
		CommonData: CommonData{
			Signature: common.Bytes2Hex(abiResult.Method.ID),
			Method:    abiResult.Method.Sig,
			Input:     txInput,
		},
		Index:       txIndex,
		Receipt:     receipt,
		Transaction: tx,
		Protected:   tx.Protected(),
		Hash:        tx.Hash().String(),
		Events:      txEvents,
	}

	return ptx, nil
}

// printDecodedTXData prints decoded txn data
func (m *Client) printDecodedTXData(l zerolog.Logger, ptx *DecodedTransaction) {
	l.Debug().Str("Method signature", ptx.Signature).Send()
	l.Debug().Str("Method name", ptx.Method).Send()
	if ptx.Input != nil {
		l.Debug().Interface("Inputs", ptx.Input).Send()
	}
	if ptx.Output != nil {
		l.Debug().Interface("Outputs", ptx.Output).Send()
	}
	for _, e := range ptx.Events {
		l.Debug().
			Str("Signature", e.Signature).
			Interface("Log", e.EventData).Send()
	}
}

// DecodeCustomABIErr decodes typed Solidity errors
func (m *Client) DecodeCustomABIErr(txErr error) (string, error) {
	//nolint
	cerr, ok := txErr.(rpc.DataError)
	if !ok {
		return "", errors.New(ErrRPCJSONCastError)
	}
	if m.ContractStore == nil {
		L.Warn().Msg(WarnNoContractStore)
		return "", nil
	}
	if cerr.ErrorData() != nil {
		L.Trace().Msg("Decoding custom ABI error from tx error")
		for _, a := range m.ContractStore.ABIs {
			for k, abiError := range a.Errors {
				data, err := hex.DecodeString(cerr.ErrorData().(string)[2:])
				if err != nil {
					return "", err
				}
				if len(data) < 4 {
					return "", err
				}
				if bytes.Equal(data[:4], abiError.ID.Bytes()[:4]) {
					// Found a matching error
					v, err := abiError.Unpack(data)
					if err != nil {
						return "", err
					}
					L.Trace().Interface("Error", k).Interface("Args", v).Msg("Revert Reason")
					return fmt.Sprintf("error type: %s, error values: %v", k, v), nil
				}
			}
		}
	} else {
		L.Debug().Msg("Transaction submission error doesn't contain any data. Impossible to decode the revert reason")
	}
	return "", nil
}

// CallMsgFromTx creates ethereum.CallMsg from tx, used in simulated calls
func (m *Client) CallMsgFromTx(tx *types.Transaction) (ethereum.CallMsg, error) {
	signer := types.LatestSignerForChainID(tx.ChainId())
	sender, err := types.Sender(signer, tx)
	if err != nil {
		return ethereum.CallMsg{}, errors.Wrapf(err, "failed to get sender from transaction")
	}

	if tx.Type() == types.LegacyTxType {
		return ethereum.CallMsg{
			From:     sender,
			To:       tx.To(),
			Gas:      tx.Gas(),
			GasPrice: tx.GasPrice(),
			Value:    tx.Value(),
			Data:     tx.Data(),
		}, nil
	}
	return ethereum.CallMsg{
		From:      sender,
		To:        tx.To(),
		Gas:       tx.Gas(),
		GasFeeCap: tx.GasFeeCap(),
		GasTipCap: tx.GasTipCap(),
		Value:     tx.Value(),
		Data:      tx.Data(),
	}, nil
}

// DownloadContractAndGetPragma retrieves the bytecode of a contract at a specified address and block,
// then decodes it to extract the pragma version. Returns the pragma version or an error if retrieval or decoding fails.
func (m *Client) DownloadContractAndGetPragma(address common.Address, block *big.Int) (Pragma, error) {
	bytecode, err := m.Client.CodeAt(context.Background(), address, block)
	if err != nil {
		return Pragma{}, errors.Wrap(err, "failed to get contract code")
	}

	pragma, err := DecodePragmaVersion(common.Bytes2Hex(bytecode))
	if err != nil {
		return Pragma{}, err
	}

	return pragma, nil
}

// callAndGetRevertReason executes transaction locally and gets revert reason
func (m *Client) callAndGetRevertReason(tx *types.Transaction, rc *types.Receipt) error {
	L.Trace().Msg("Decoding revert error")
	// bind should support custom errors decoding soon, not yet merged
	// https://github.com/ethereum/go-ethereum/issues/26823
	// there are 2 types of possible errors, plain old assert/revert string
	// or new ABI encoded errors, first we try to find ABI one
	// if there is no match we print the error from CallMsg call
	msg, err := m.CallMsgFromTx(tx)
	if err != nil {
		L.Debug().Msgf("Failed to extract required data from transaction due to: %s, We won't be able to decode revert reason.", err.Error())
		return nil
	}
	_, plainStringErr := m.Client.CallContract(context.Background(), msg, rc.BlockNumber)

	decodedABIErrString, err := m.DecodeCustomABIErr(plainStringErr)
	if err != nil {
		return err
	}
	if decodedABIErrString != "" {
		return errors.New(decodedABIErrString)
	}

	if plainStringErr != nil {
		L.Debug().Msg("Failed to decode revert reason")

		if plainStringErr.Error() == "execution reverted" && tx != nil && rc != nil {
			if tx.To() != nil {
				pragma, err := m.DownloadContractAndGetPragma(*tx.To(), rc.BlockNumber)
				if err == nil {
					if DoesPragmaSupportCustomRevert(pragma) {
						L.Warn().Str("Pragma", fmt.Sprint(pragma)).Msg("Custom revert reason is supported by pragma, but we could not decode it. If you are sure that this contract has custom revert reasons this might indicate a bug in Seth. Please contact the Test Tooling team.")
					} else {
						L.Info().Str("Pragma", fmt.Sprint(pragma)).Msg("Custom revert reason is not supported by pragma version (must be >= 0.8.4). There's nothing more we can do to get custom revert reason.")
					}
				} else {
					L.Debug().Msgf("Failed to decode pragma version due to: %s. Contract either uses very old version or was compiled without metadata. We won't be able to decode revert reason.", err.Error())
				}
			} else {
				L.Debug().Msg("Transaction has no recipient address. Most likely it's a contract creation transaction. We don't support decoding revert reasons for contract creation transactions yet.")
			}
		}

		return plainStringErr
	}
	return nil
}

// decodeTxInputs decoded tx inputs
func decodeTxInputs(l zerolog.Logger, txData []byte, method *abi.Method) (map[string]interface{}, error) {
	l.Trace().Msg("Parsing tx inputs")
	if (len(txData)) < 4 {
		return nil, errors.New(ErrTooShortTxData)
	}

	inputMap := make(map[string]interface{})
	payload := txData[4:]
	if len(payload) == 0 || len(method.Inputs) == 0 {
		return nil, nil
	}
	err := method.Inputs.UnpackIntoMap(inputMap, payload)
	if err != nil {
		return nil, err
	}
	l.Trace().Interface("Inputs", inputMap).Msg("Transaction inputs")
	return inputMap, nil
}

// decodeTxOutputs decoded tx outputs
func decodeTxOutputs(l zerolog.Logger, payload []byte, method *abi.Method) (map[string]interface{}, error) {
	l.Trace().Msg("Parsing tx outputs")
	outputMap := make(map[string]interface{})
	// unpack method outputs
	if len(payload) == 0 {
		return nil, nil
	}
	// TODO: is it possible to have both anonymous and non-anonymous fields in solidity?
	if len(method.Outputs) > 0 && method.Outputs[0].Name == "" {
		vals, err := method.Outputs.UnpackValues(payload)
		if err != nil {
			return nil, err
		}
		for i, v := range vals {
			outputMap[strconv.Itoa(i)] = v
		}
	} else {
		err := method.Outputs.UnpackIntoMap(outputMap, payload)
		if err != nil {
			return nil, errors.Wrap(err, ErrDecodeOutput)
		}
	}
	l.Trace().Interface("Outputs", outputMap).Msg("Transaction outputs")
	return outputMap, nil
}

type DecodableLog interface {
	GetTopics() []common.Hash
	GetData() []byte
}

// decodeEventFromLog parses log body and additional topic fields
func decodeEventFromLog(
	l zerolog.Logger,
	a abi.ABI,
	eventABISpec abi.Event,
	lo DecodableLog,
) (map[string]interface{}, map[string]interface{}, error) {
	eventsMap := make(map[string]interface{})
	topicsMap := make(map[string]interface{})
	// if no data event has only indexed fields
	if len(lo.GetData()) != 0 {
		err := a.UnpackIntoMap(eventsMap, eventABISpec.Name, lo.GetData())
		if err != nil {
			return nil, nil, errors.Wrap(err, ErrDecodedLogNonIndexed)
		}
		l.Trace().Interface("Non-indexed", eventsMap).Send()
	}
	// might have up to 3 additional indexed fields
	if len(lo.GetTopics()) > 1 {
		topics := lo.GetTopics()[1:]
		var indexed []abi.Argument
		indexedTopics := make([]common.Hash, 0)
		for idx, topic := range topics {
			arg := eventABISpec.Inputs[idx]
			if arg.Indexed {
				indexed = append(indexed, arg)
				indexedTopics = append(indexedTopics, topic)
			}
		}
		l.Trace().Int("Topics", len(lo.GetTopics()[1:])).Int("Arguments", len(indexed)).Send()
		l.Trace().Interface("AllTopics", lo.GetTopics()).Send()
		l.Trace().Interface("HashOfName", eventABISpec.ID.Hex()).Send()
		l.Trace().Interface("FirstTopic", lo.GetTopics()[0]).Send()
		l.Trace().Interface("Topics", lo.GetTopics()[1:]).Send()
		l.Trace().Interface("Arguments", eventABISpec.Inputs).Send()
		l.Trace().Interface("Indexed", indexed).Send()
		err := abi.ParseTopicsIntoMap(topicsMap, indexed, indexedTopics)
		if err != nil {
			return nil, nil, errors.Wrap(err, ErrDecodeILogIndexed)
		}
		l.Trace().Interface("Indexed", topicsMap).Send()
	}
	return eventsMap, topicsMap, nil
}

type LogWithEventData interface {
	MergeEventData(map[string]interface{})
}

// decodedLogFromMaps creates DecodedLog from events and topics maps
func decodedLogFromMaps(log LogWithEventData, eventsMap map[string]interface{}, topicsMap map[string]interface{}) LogWithEventData {
	newMap := map[string]interface{}{}
	for k, v := range eventsMap {
		newMap[k] = v
	}
	for k, v := range topicsMap {
		newMap[k] = v
	}

	log.MergeEventData(newMap)

	return log
}
