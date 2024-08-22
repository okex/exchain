package types

import (
	"encoding/binary"
	"fmt"
	"time"

	cryptoAmino "github.com/okex/exchain/libs/tendermint/crypto/encoding/amino"

	"github.com/okex/exchain/libs/tendermint/crypto"
	"github.com/okex/exchain/libs/tendermint/libs/bech32"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
)

const (
	// ModuleName is the name of the staking module
	ModuleName = "staking"

	// StoreKey is the string store representation
	StoreKey = ModuleName

	// TStoreKey is the string transient store representation
	TStoreKey = "transient_" + ModuleName

	// QuerierRoute is the querier route for the staking module
	QuerierRoute = ModuleName

	// RouterKey is the msg router key for the staking module
	RouterKey = ModuleName
)

// nolint
var (
	// Keys for store prefixes
	// Last* values are constant during a block.
	LastValidatorPowerKey = []byte{0x11} // prefix for each key to a validator index, for bonded validators
	LastTotalPowerKey     = []byte{0x12} // prefix for the total power

	ValidatorsKey             = []byte{0x21} // prefix for each key to a validator
	ValidatorsByConsAddrKey   = []byte{0x22} // prefix for each key to a validator index, by pubkey
	ValidatorsByPowerIndexKey = []byte{0x23} // prefix for each key to a validator index, sorted by power
	ValChangePubkeyKey        = []byte{0x24}

	ValidatorQueueKey = []byte{0x43} // prefix for the timestamps in validator queue

	SharesKey           = []byte{0x51}
	DelegatorKey        = []byte{0x52}
	UnDelegationInfoKey = []byte{0x53}
	UnDelegateQueueKey  = []byte{0x54}
	ProxyKey            = []byte{0x55}

	// prefix key for vals info to enforce the update of validator-set
	ValidatorAbandonedKey = []byte{0x60}

	lenTime = len(sdk.FormatTimeBytes(time.Now()))
)

// GetValidatorKey gets the key for the validator with address
// VALUE: staking/Validator
func GetValidatorKey(operatorAddr sdk.ValAddress) []byte {
	return append(ValidatorsKey, operatorAddr.Bytes()...)
}

// GetValidatorByConsAddrKey gets the key for the validator with pubkey
// VALUE: validator operator address ([]byte)
func GetValidatorByConsAddrKey(addr sdk.ConsAddress) []byte {
	return append(ValidatorsByConsAddrKey, addr.Bytes()...)
}

func GetValidatorChangePubkeyKey(operatorAddr sdk.ValAddress) []byte {
	return append(ValChangePubkeyKey, operatorAddr.Bytes()...)
}

// AddressFromLastValidatorPowerKey gets the validator operator address from LastValidatorPowerKey
func AddressFromLastValidatorPowerKey(key []byte) []byte {
	return key[1:] // remove prefix bytes
}

// GetValidatorsByPowerIndexKey gets the validator by power index
// Power index is the key used in the power-store, and represents the relative power ranking of the validator
// VALUE: validator operator address ([]byte)
func GetValidatorsByPowerIndexKey(validator Validator) []byte {
	// NOTE the address doesn't need to be stored because counter bytes must always be different
	return getValidatorPowerRank(validator)
}

// GetLastValidatorPowerKey gets the bonded validator index key for an operator address
func GetLastValidatorPowerKey(operator sdk.ValAddress) []byte {
	return append(LastValidatorPowerKey, operator...)
}

// GetValidatorQueueTimeKey gets the prefix for all unbonding delegations from a delegator
func GetValidatorQueueTimeKey(timestamp time.Time) []byte {
	bz := sdk.FormatTimeBytes(timestamp)
	return append(ValidatorQueueKey, bz...)
}

// getValidatorPowerRank gets the power ranking of a validator by okexchain's rule
// just according to the shares instead of tokens on a validator
func getValidatorPowerRank(validator Validator) []byte {
	// consensus power based on the shares on a validator
	consensusPower := sharesToConsensusPower(validator.DelegatorShares)
	consensusPowerBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(consensusPowerBytes[:], uint64(consensusPower))

	powerBytes := consensusPowerBytes
	powerBytesLen := len(powerBytes) // 8

	// key is of format prefix || powerbytes || addrBytes
	key := make([]byte, 1+powerBytesLen+sdk.AddrLen)

	key[0] = ValidatorsByPowerIndexKey[0]
	copy(key[1:powerBytesLen+1], powerBytes)
	operAddrInvr := sdk.CopyBytes(validator.OperatorAddress)
	for i, b := range operAddrInvr {
		operAddrInvr[i] = ^b
	}
	copy(key[powerBytesLen+1:], operAddrInvr)

	return key
}

// GetDelegatorKey gets the key for Delegator
func GetDelegatorKey(delAddr sdk.AccAddress) []byte {
	return append(DelegatorKey, delAddr.Bytes()...)
}

// GetProxyDelegatorKey gets the key for the relationship between delegator and proxy
func GetProxyDelegatorKey(proxyAddr, delAddr sdk.AccAddress) []byte {
	return append(append(ProxyKey, proxyAddr...), delAddr...)
}

// GetSharesKey gets the whole key for an item of shares info
func GetSharesKey(valAddr sdk.ValAddress, delAddr sdk.AccAddress) []byte {
	return append(GetSharesToValidatorsKey(valAddr), delAddr.Bytes()...)
}

// GetSharesToValidatorsKey gets the first-prefix for an item of shares info
func GetSharesToValidatorsKey(valAddr sdk.ValAddress) []byte {
	return append(SharesKey, valAddr.Bytes()...)
}

// GetUndelegationInfoKey gets the key for UndelegationInfo
func GetUndelegationInfoKey(delAddr sdk.AccAddress) []byte {
	return append(UnDelegationInfoKey, delAddr.Bytes()...)
}

// GetCompleteTimeKey get the key for the prefix of time
func GetCompleteTimeKey(timestamp time.Time) []byte {
	bz := sdk.FormatTimeBytes(timestamp)
	return append(UnDelegateQueueKey, bz...)
}

// GetCompleteTimeWithAddrKey get the key for the complete time with delegator address
func GetCompleteTimeWithAddrKey(timestamp time.Time, delAddr sdk.AccAddress) []byte {
	return append(GetCompleteTimeKey(timestamp), delAddr.Bytes()...)
}

// SplitCompleteTimeWithAddrKey splits the key and returns the endtime and delegator address
func SplitCompleteTimeWithAddrKey(key []byte) (time.Time, sdk.AccAddress) {
	if len(key[1:]) != lenTime+sdk.AddrLen {
		panic(fmt.Sprintf("unexpected key length (%d ≠ %d)", len(key[1:]), lenTime+sdk.AddrLen))
	}
	endTime, err := sdk.ParseTimeBytes(key[1 : 1+lenTime])
	if err != nil {
		panic(err)
	}
	delAddr := sdk.AccAddress(key[1+lenTime:])
	return endTime, delAddr
}

// Bech32ifyConsPub returns a Bech32 encoded string containing the
// Bech32PrefixConsPub prefixfor a given consensus node's PubKey.
func Bech32ifyConsPub(pub crypto.PubKey) (string, error) {
	bech32PrefixConsPub := sdk.GetConfig().GetBech32ConsensusPubPrefix()
	return bech32.ConvertAndEncode(bech32PrefixConsPub, pub.Bytes())
}

func MustBech32ifyConsPub(pub crypto.PubKey) string {
	enc, err := Bech32ifyConsPub(pub)
	if err != nil {
		panic(err)
	}

	return enc
}

// GetConsPubKeyBech32 creates a PubKey for a consensus node with a given public
// key string using the Bech32 Bech32PrefixConsPub prefix.
func GetConsPubKeyBech32(pubkey string) (pk crypto.PubKey, err error) {
	bech32PrefixConsPub := sdk.GetConfig().GetBech32ConsensusPubPrefix()
	bz, err := sdk.GetFromBech32(pubkey, bech32PrefixConsPub)
	if err != nil {
		return nil, err
	}

	pk, err = cryptoAmino.PubKeyFromBytes(bz)
	if err != nil {
		return nil, err
	}

	return pk, nil
}

// MustGetConsPubKeyBech32 returns the result of GetConsPubKeyBech32 panicing on
// failure.
func MustGetConsPubKeyBech32(pubkey string) (pk crypto.PubKey) {
	pk, err := GetConsPubKeyBech32(pubkey)
	if err != nil {
		panic(err)
	}

	return pk
}
