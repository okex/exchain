package types

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/vm"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"math/big"
)

var (
	DefaultGuFactor = sdk.NewDec(-1)
)

type GuFactor struct {
	Factor sdk.Dec `json:"gu_factor" yaml:"gu_factor"`
}

func (factor GuFactor) ValidateBasic() error {
	// Factor must [0,...)
	if factor.Factor.LT(sdk.ZeroDec()) {
		return ErrGUFactor
	}
	return nil
}

func UnmarshalGuFactor(data string) (GuFactor, error) {
	var factor GuFactor
	err := json.Unmarshal([]byte(data), &factor)
	if factor.Factor.IsNil() {
		return factor, fmt.Errorf("json unmarshal failed: %v", err)
	}
	return factor, err
}

type GuFactorHook struct {
}

func (hook GuFactorHook) UpdateGuFactor(csdb *CommitStateDB, op vm.OpCode, from, to common.Address, input []byte, value *big.Int) {
	if op == vm.CALL || op == vm.DELEGATECALL || op == vm.CALLCODE {
		if len(input) > 4 {
			input = input[:4]
		}
		method := hexutil.Encode(input)

		if bc := csdb.GetContractMethodBlockedByAddress(to.Bytes()); bc != nil {
			if contractMethod := bc.BlockMethods.GetMethod(method); contractMethod != nil {
				if factor := contractMethod.GetGuFactor(); factor != nil {
					if factor.Factor.GT(csdb.GuFactor) {
						csdb.GuFactor = factor.Factor
					}
				}
			}
		}
	}
}
