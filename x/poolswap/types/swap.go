package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/supply"
	token "github.com/okex/okchain/x/token/types"

	"fmt"
	"regexp"
	"strings"
)

// PoolTokenPrefix defines pool token prefix name
const PoolTokenPrefix = "poolswap-"

// SwapTokenPair defines token pair exchange
type SwapTokenPair struct {
	QuotePooledCoin sdk.DecCoin `json:"quote_pooled_coin"` // The volume of quote token in the token pair exchange pool
	BasePooledCoin  sdk.DecCoin `json:"base_pooled_coin"`  // The volume of base token in the token pair exchange pool
	PoolTokenName   string      `json:"pool_token_name"`   // The name of pool token
}

// NewSwapTokenPair is a constructor function for SwapTokenPair
func NewSwapTokenPair(quotePooledCoin sdk.DecCoin, basePooledCoin sdk.DecCoin, poolTokenName string) *SwapTokenPair {
	swapTokenPair := &SwapTokenPair{
		QuotePooledCoin: quotePooledCoin,
		BasePooledCoin:  basePooledCoin,
		PoolTokenName:   poolTokenName,
	}
	return swapTokenPair
}

// String implement fmt.Stringer
func (s SwapTokenPair) String() string {
	return strings.TrimSpace(fmt.Sprintf(`QuotePooledCoin: %s
BasePooledCoin: %s
PoolTokenName: %s`, s.QuotePooledCoin.String(), s.BasePooledCoin.String(), s.PoolTokenName))
}

// TokenPairName defines token pair
func (s SwapTokenPair) TokenPairName() string {
	return s.BasePooledCoin.Denom + "_" + s.QuotePooledCoin.Denom
}

// InitPoolToken default pool token
func InitPoolToken(poolTokenName string) token.Token {
	return token.Token{
		Description:         poolTokenName,
		Symbol:              poolTokenName,
		OriginalSymbol:      poolTokenName,
		WholeName:           poolTokenName,
		OriginalTotalSupply: sdk.NewDec(0),
		TotalSupply:         sdk.NewDec(0),
		Owner:               supply.NewModuleAddress(ModuleName),
		Type:                GenerateTokenType,
		Mintable:            true,
	}
}

// ValidatePoolTokenName validates the format of specified pool token name
func ValidatePoolTokenName(tokenName string) bool {
	var poolTokenFormat = fmt.Sprintf(`^(%s)[a-z][a-z0-9]{0,9}(\-[a-f0-9]{3})?$`, PoolTokenPrefix)
	var poolTokenRegExp = regexp.MustCompile(poolTokenFormat)
	return poolTokenRegExp.MatchString(tokenName)
}
