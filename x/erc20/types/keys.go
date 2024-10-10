package types

const (
	// ModuleName string name of module
	ModuleName = "erc20"
	// StoreKey key for ethereum storage data, account code (StateDB) or block
	// related data for Web3.
	// The erc20 module should use a prefix store.
	StoreKey = ModuleName
	// RouterKey uses module name for routing
	RouterKey = ModuleName

	QueryParameters          = "params"
	QueryTokenMapping        = "token-mapping"
	QueryContractByDenom     = "contract-by-denom"
	QueryDenomByContract     = "denom-by-contract"
	QueryContractTem         = "current-template-contract"
	QueryTokenMappingChannel = "token-mapping-channel"
)

// KVStore key prefixes
var (
	KeyPrefixContractToDenom  = []byte{0x01}
	KeyPrefixDenomToContract  = []byte{0x02}
	KeyPrefixTemplateContract = []byte{0x03}
)

// ContractToDenomKey defines the store key for contract to denom reverse index
func ContractToDenomKey(contract []byte) []byte {
	return append(KeyPrefixContractToDenom, contract...)
}

// DenomToContractKey defines the store key for denom to contract mapping
func DenomToContractKey(denom string) []byte {
	return append(KeyPrefixDenomToContract, denom...)
}

func ConstructContractKey(str string) []byte {
	return append(KeyPrefixTemplateContract, []byte(str)...)
}
