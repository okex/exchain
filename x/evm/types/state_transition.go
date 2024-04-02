package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	types2 "github.com/okex/exchain/libs/cosmos-sdk/store/types"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	"github.com/okex/exchain/libs/cosmos-sdk/types/innertx"
	"github.com/okex/exchain/libs/system/trace"
	"github.com/okex/exchain/libs/tendermint/types"
)

// StateTransition defines data to transitionDB in evm
type StateTransition struct {
	// TxData fields
	AccountNonce uint64
	Price        *big.Int
	GasLimit     uint64
	Recipient    *common.Address
	Amount       *big.Int
	Payload      []byte

	ChainID    *big.Int
	Csdb       *CommitStateDB
	TxHash     *common.Hash
	Sender     common.Address
	Simulate   bool // i.e CheckTx execution
	TraceTx    bool // reexcute tx or its predesessors
	TraceTxLog bool // trace tx for its evm logs (predesessors are set to false)
	callToCM   vm.CallToWasmByPrecompile
}

// GasInfo returns the gas limit, gas consumed and gas refunded from the EVM transition
// execution
type GasInfo struct {
	GasLimit    uint64
	GasConsumed uint64
}

// ExecutionResult represents what's returned from a transition
type ExecutionResult struct {
	Logs      []*ethtypes.Log
	Bloom     *big.Int
	Result    *sdk.Result
	GasInfo   GasInfo
	TraceLogs []byte
}

// GetHashFn implements vm.GetHashFunc for Ethermint. It handles 3 cases:
//  1. The requested height matches the current height (and thus same epoch number)
//  2. The requested height is from an previous height from the same chain epoch
//  3. The requested height is from a height greater than the latest one
func GetHashFn(ctx sdk.Context, csdb *CommitStateDB) vm.GetHashFunc {
	return func(height uint64) common.Hash {
		switch {
		case ctx.BlockHeight() == int64(height):
			// Case 1: The requested height matches the one from the context so we can retrieve the header
			// hash directly from the context.
			return csdb.bhash

		case ctx.BlockHeight() > int64(height):
			// Case 2: if the chain is not the current height we need to retrieve the hash from the store for the
			// current chain epoch. This only applies if the current height is greater than the requested height.
			return csdb.WithContext(ctx).GetHeightHash(height)

		default:
			// Case 3: heights greater than the current one returns an empty hash.
			return common.Hash{}
		}
	}
}

func (st *StateTransition) newEVM(
	ctx sdk.Context,
	csdb *CommitStateDB,
	gasLimit uint64,
	gasPrice *big.Int,
	config *ChainConfig,
	vmConfig vm.Config,
) *vm.EVM {
	// Create context for evm
	blockCtx := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		GetHash:     GetHashFn(ctx, csdb),
		Coinbase:    common.BytesToAddress(ctx.BlockProposerAddress()),
		BlockNumber: big.NewInt(ctx.BlockHeight()),
		Time:        big.NewInt(ctx.BlockTime().Unix()),
		Difficulty:  big.NewInt(0), // unused. Only required in PoW context
		GasLimit:    gasLimit,
	}
	ctx.SetEVMStateDB(st.Csdb)
	txCtx := vm.TxContext{
		Origin:    st.Sender,
		GasPrice:  gasPrice,
		OKContext: &ctx,
		CallToCM:  st.GetCallToCM(),
	}

	return vm.NewEVM(blockCtx, txCtx, csdb, config.EthereumConfig(st.ChainID), vmConfig)
}

func (st *StateTransition) applyOverrides(ctx sdk.Context, csdb *CommitStateDB) error {
	overrideBytes := ctx.OverrideBytes()
	if overrideBytes != nil {
		var stateOverrides StateOverrides
		err := json.Unmarshal(overrideBytes, &stateOverrides)
		if err != nil {
			return fmt.Errorf("failed to decode stateOverrides")
		}
		stateOverrides.Apply(csdb)
	}
	return nil
}

// TransitionDb will transition the state by applying the current transaction and
// returning the evm execution result.
// NOTE: State transition checks are run during AnteHandler execution.
func (st StateTransition) TransitionDb(ctx sdk.Context, config ChainConfig) (exeRes *ExecutionResult, resData *ResultData, err error, innerTxs, erc20Contracts interface{}) {
	preSSId := st.Csdb.Snapshot()
	contractCreation := st.Recipient == nil

	defer func() {
		if e := recover(); e != nil {
			if !st.Simulate {
				st.Csdb.RevertToSnapshot(preSSId)
			}

			// if the msg recovered can be asserted into type 'ErrContractBlockedVerify', it must be captured by the panics of blocked
			// contract calling
			switch rType := e.(type) {
			case ErrContractBlockedVerify:
				err = ErrCallBlockedContract(rType.Descriptor)
			default:
				panic(e)
			}
		}
	}()

	cost, err := core.IntrinsicGas(st.Payload, []ethtypes.AccessTuple{}, contractCreation, config.IsHomestead(), config.IsIstanbul())
	if err != nil {
		return exeRes, resData, sdkerrors.Wrap(err, "invalid intrinsic gas for transaction"), innerTxs, erc20Contracts
	}

	consumedGas := ctx.GasMeter().GasConsumed()
	if consumedGas < cost {
		// If Cosmos standard tx ante handler cost is less than EVM intrinsic cost
		// gas must be consumed to match to accurately simulate an Ethereum transaction
		ctx.GasMeter().ConsumeGas(cost-consumedGas, "Intrinsic gas match")
	}

	// This gas limit the transaction gas limit with intrinsic gas subtracted
	gasLimit := st.GasLimit - ctx.GasMeter().GasConsumed()

	// This gas meter is set up to consume gas from gaskv during evm execution and be ignored
	currentGasMeter := ctx.GasMeter()
	evmGasMeter := sdk.NewInfiniteGasMeter()
	ctx.SetGasMeter(evmGasMeter)
	csdb := st.Csdb.WithContext(ctx)

	StartTxLog := func(tag string) {
		if !ctx.IsCheckTx() {
			trace.StartTxLog(tag)
		}
	}
	StopTxLog := func(tag string) {
		if !ctx.IsCheckTx() {
			trace.StopTxLog(tag)
		}
	}
	if ctx.IsCheckTx() {
		if err = st.applyOverrides(ctx, csdb); err != nil {
			return
		}
	}

	params := csdb.GetParams()

	var senderStr = EthAddressToString(&st.Sender)

	to := ""
	var recipientStr string
	if st.Recipient != nil {
		to = EthAddressToString(st.Recipient)
		recipientStr = to
	}
	tracer := newTracer(ctx, st.TxHash)
	vmConfig := vm.Config{
		ExtraEips:               params.ExtraEIPs,
		Debug:                   st.TraceTxLog,
		Tracer:                  tracer,
		ContractVerifier:        NewContractVerifier(params),
		EnablePreimageRecording: st.TraceTxLog,
	}

	evm := st.newEVM(ctx, csdb, gasLimit, st.Price, &config, vmConfig)

	var (
		ret             []byte
		leftOverGas     uint64
		contractAddress common.Address
		recipientLog    string
		senderRef       = vm.AccountRef(st.Sender)
		gasConsumed     uint64
	)

	// Get nonce of account outside of the EVM
	currentNonce := csdb.GetNonce(st.Sender)
	// Set nonce of sender account before evm state transition for usage in generating Create address
	csdb.SetNonce(st.Sender, st.AccountNonce)

	//add InnerTx
	callTx := innertx.AddDefaultInnerTx(evm, innertx.CosmosDepth, senderStr, "", "", "", st.Amount, nil, st.Payload)

	// create contract or execute call
	switch contractCreation {
	case true:
		if !params.EnableCreate {
			if !st.Simulate {
				st.Csdb.RevertToSnapshot(preSSId)
			}

			return exeRes, resData, ErrCreateDisabled, innerTxs, erc20Contracts
		}

		// check whether the deployer address is in the whitelist if the whitelist is enabled
		senderAccAddr := st.Sender.Bytes()
		if params.EnableContractDeploymentWhitelist && !csdb.IsDeployerInWhitelist(senderAccAddr) {
			if !st.Simulate {
				st.Csdb.RevertToSnapshot(preSSId)
			}

			return exeRes, resData, ErrUnauthorizedAccount(senderAccAddr), innerTxs, erc20Contracts
		}

		StartTxLog(trace.EVMCORE)
		defer StopTxLog(trace.EVMCORE)
		nonce := evm.StateDB.GetNonce(st.Sender)
		ret, contractAddress, leftOverGas, err = evm.Create(senderRef, st.Payload, gasLimit, st.Amount)

		contractAddressStr := EthAddressToString(&contractAddress)
		recipientLog = strings.Join([]string{"contract address ", contractAddressStr}, "")
		gasConsumed = gasLimit - leftOverGas
		if !ctx.IsMempoolSimulate() && !csdb.GuFactor.IsNegative() {
			gasConsumed = csdb.GuFactor.MulInt(sdk.NewIntFromUint64(gasConsumed)).TruncateInt().Uint64()
		}
		//if no err, we must be check weather out of gas because, we may increase gasConsumed by 'csdb.GuFactor'.
		if err == nil {
			if gasLimit < gasConsumed {
				err = vm.ErrOutOfGas
				//if out of gas,then err is ErrOutOfGas, gasConsumed change to gasLimit for can not make line.295 panic that will lead to 'RevertToSnapshot' panic
				gasConsumed = gasLimit
			}
		} else {
			if gasConsumed > gasLimit {
				gasConsumed = gasLimit
				defer func() {
					panic(types2.ErrorOutOfGas{"EVM execution consumption"})
				}()
			}
		}
		innertx.UpdateDefaultInnerTx(callTx, contractAddressStr, innertx.CosmosCallType, innertx.EvmCreateName, gasConsumed, nonce)
	default:
		if !params.EnableCall {
			if !st.Simulate {
				st.Csdb.RevertToSnapshot(preSSId)
			}

			return exeRes, resData, ErrCallDisabled, innerTxs, erc20Contracts
		}

		// Increment the nonce for the next transaction	(just for evm state transition)
		csdb.SetNonce(st.Sender, csdb.GetNonce(st.Sender)+1)
		StartTxLog(trace.EVMCORE)
		defer StopTxLog(trace.EVMCORE)
		ret, leftOverGas, err = evm.Call(senderRef, *st.Recipient, st.Payload, gasLimit, st.Amount)

		if recipientStr == "" {
			recipientStr = EthAddressToString(st.Recipient)
		}

		recipientLog = strings.Join([]string{"recipient address ", recipientStr}, "")
		gasConsumed = gasLimit - leftOverGas
		if !ctx.IsMempoolSimulate() && !csdb.GuFactor.IsNegative() {
			gasConsumed = csdb.GuFactor.MulInt(sdk.NewIntFromUint64(gasConsumed)).TruncateInt().Uint64()
		}
		//if no err, we must be check weather out of gas because, we may increase gasConsumed by 'csdb.GuFactor'.
		if err == nil {
			if gasLimit < gasConsumed {
				err = vm.ErrOutOfGas
				//if out of gas,then err is ErrOutOfGas, gasConsumed change to gasLimit for can not make line.295 panic that will lead to 'RevertToSnapshot' panic
				gasConsumed = gasLimit
			}
		} else {
			// For cover err != nil,but gasConsumed which is caculated by gufactor  >  gaslimit,we must be make gasConsumed = gasLimit and panic same as currentGasMeter.ConsumeGas. so we can not use height isolation
			if gasConsumed > gasLimit {
				gasConsumed = gasLimit
				defer func() {
					panic(types2.ErrorOutOfGas{"EVM execution consumption"})
				}()
			}
		}

		innertx.UpdateDefaultInnerTx(callTx, recipientStr, innertx.CosmosCallType, innertx.EvmCallName, gasConsumed, 0)
	}

	innerTxs, erc20Contracts = innertx.ParseInnerTxAndContract(evm, err != nil)

	defer func() {
		// Consume gas from evm execution
		// Out of gas check does not need to be done here since it is done within the EVM execution
		currentGasMeter.ConsumeGas(gasConsumed, "EVM execution consumption")
	}()

	// return trace log if tracetxlog no matter err = nil  or not nil
	defer func() {
		var traceLogs []byte
		var traceErr error
		if st.TraceTxLog {
			result := &core.ExecutionResult{
				UsedGas:    gasConsumed,
				Err:        err,
				ReturnData: ret,
			}
			traceLogs, traceErr = GetTracerResult(tracer, result)
			if traceErr != nil {
				traceLogs = []byte(traceErr.Error())
			} else {
				traceLogs, traceErr = integratePreimage(csdb, traceLogs)
				if traceErr != nil {
					traceLogs = []byte(traceErr.Error())
				}
			}
			if exeRes == nil {
				exeRes = &ExecutionResult{
					Result: &sdk.Result{},
				}
			}
			exeRes.TraceLogs = traceLogs
		}
	}()
	if err != nil {
		if !st.Simulate {
			st.Csdb.RevertToSnapshot(preSSId)
		}

		// Consume gas before returning
		return exeRes, resData, newRevertError(ret, err), innerTxs, erc20Contracts
	}

	// Resets nonce to value pre state transition
	csdb.SetNonce(st.Sender, currentNonce)

	// Generate bloom filter to be saved in tx receipt data
	bloomInt := big.NewInt(0)

	var (
		bloomFilter ethtypes.Bloom
		logs        []*ethtypes.Log
	)

	if st.TxHash != nil {
		logs, err = csdb.GetLogs(*st.TxHash)
		if err != nil {
			st.Csdb.RevertToSnapshot(preSSId)
			return
		}

		bloomInt = big.NewInt(0).SetBytes(ethtypes.LogsBloom(logs))
		bloomFilter = ethtypes.BytesToBloom(bloomInt.Bytes())
	}

	if !st.Simulate {
		if types.HigherThanMars(ctx.BlockHeight()) {
			if ctx.IsDeliverWithSerial() {
				csdb.IntermediateRoot(true)
			}
		} else {
			csdb.Commit(true)
		}
	}

	// Encode all necessary data into slice of bytes to return in sdk result
	resData = &ResultData{
		Bloom:  bloomFilter,
		Logs:   logs,
		Ret:    ret,
		TxHash: *st.TxHash,
	}

	if contractCreation {
		resData.ContractAddress = contractAddress
	}

	resBz, err := EncodeResultData(resData)
	if err != nil {
		return
	}

	resultLog := strings.Join([]string{"executed EVM state transition; sender address ", senderStr, "; ", recipientLog}, "")
	exeRes = &ExecutionResult{
		Logs:  logs,
		Bloom: bloomInt,
		Result: &sdk.Result{
			Data: resBz,
			Log:  resultLog,
		},
		GasInfo: GasInfo{
			GasConsumed: gasConsumed,
			GasLimit:    gasLimit,
		},
	}
	return
}

func newRevertError(data []byte, e error) error {
	var resultError []string
	if data == nil || e.Error() != vm.ErrExecutionReverted.Error() {
		return e
	}
	resultError = append(resultError, e.Error())
	reason, errUnpack := abi.UnpackRevert(data)
	if errUnpack == nil {
		resultError = append(resultError, vm.ErrExecutionReverted.Error()+":"+reason)
	} else {
		resultError = append(resultError, hexutil.Encode(data))
	}
	resultError = append(resultError, ErrorHexData)
	resultError = append(resultError, hexutil.Encode(data))
	ret, error := json.Marshal(resultError)

	//failed to marshal, return original data in error
	if error != nil {
		return fmt.Errorf(e.Error()+"[%v]", hexutil.Encode(data))
	}
	return errors.New(string(ret))
}

func integratePreimage(csdb *CommitStateDB, traceLogs []byte) ([]byte, error) {
	var traceLogsMap map[string]interface{}
	if err := json.Unmarshal(traceLogs, &traceLogsMap); err != nil {
		return nil, err
	}

	preimageMap := make(map[string]interface{})
	for k, v := range csdb.preimages {
		preimageMap[k.Hex()] = hexutil.Encode(v)
	}
	traceLogsMap["preimage"] = preimageMap
	return json.Marshal(traceLogsMap)
}

func (st *StateTransition) SetCallToCM(callToCM vm.CallToWasmByPrecompile) {
	st.callToCM = callToCM
}

func (st StateTransition) GetCallToCM() vm.CallToWasmByPrecompile {
	return st.callToCM
}
