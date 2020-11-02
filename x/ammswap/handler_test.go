package ammswap

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/supply"
	"github.com/okex/okexchain/x/ammswap/keeper"
	"github.com/okex/okexchain/x/ammswap/types"
	token "github.com/okex/okexchain/x/token/types"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
)

func TestHandleMsgCreateExchange(t *testing.T) {
	mapp, addrKeysSlice := getMockApp(t, 1)
	keeper := mapp.swapKeeper
	mapp.BeginBlock(abci.RequestBeginBlock{Header: abci.Header{Height: 2}})
	ctx := mapp.BaseApp.NewContext(false, abci.Header{}).WithBlockHeight(10)
	mapp.supplyKeeper.SetSupply(ctx, supply.NewSupply(mapp.TotalCoinsSupply))
	handler := NewHandler(keeper)

	testToken := initToken(types.TestBasePooledToken)
	testToken2 := initToken(types.TestBasePooledToken2)
	testQuoteToken := initToken(types.TestQuotePooledToken)

	mapp.tokenKeeper.NewToken(ctx, testToken)
	mapp.tokenKeeper.NewToken(ctx, testToken2)
	mapp.tokenKeeper.NewToken(ctx, testQuoteToken)

	tests := []struct {
		testCase               string
		token0                 string
		token1                 string
		addr                   sdk.AccAddress
		expectedCode           sdk.CodeType
	}{
		{
			testCase:               "token is not exist",
			token0:                 testToken.Symbol,
			token1:                 types.TestBasePooledToken3,
			addr:                   addrKeysSlice[0].Address,
			expectedCode:           sdk.CodeInternal},
		{
			testCase:               "success",
			token0:                 testToken.Symbol,
			token1:                 testQuoteToken.Symbol,
			addr:                   addrKeysSlice[0].Address,
			expectedCode:           sdk.CodeOK,},
		{
			testCase:               "success(The lexicographic order of BaseTokenName must be less than QuoteTokenName)",
			token0:                 testToken2.Symbol,
			token1:                 testToken.Symbol,
			addr:                   addrKeysSlice[0].Address,
			expectedCode:           sdk.CodeOK},
		{
			testCase:               "swapTokenPair already exists",
			token0:                 testToken.Symbol,
			token1:                 testQuoteToken.Symbol,
			addr:                   addrKeysSlice[0].Address,
			expectedCode:           sdk.CodeInternal},
	}
	for _, testCase := range tests {
		fmt.Println(testCase.testCase)
		addLiquidityMsg := types.NewMsgCreateExchange(testCase.token0, testCase.token1, testCase.addr)
		result := handler(ctx, addLiquidityMsg)
		require.Equal(t, testCase.expectedCode, result.Code)
		if result.IsOK() {
			expectedSwapTokenPairName := types.GetSwapTokenPairName(testCase.token0, testCase.token1)
			swapTokenPair, err := keeper.GetSwapTokenPair(ctx, expectedSwapTokenPairName)
			expectedBaseTokenName, expectedQuoteTokenName := types.GetBaseQuoteTokenName(testCase.token0, testCase.token1)
			require.Nil(t, err)
			require.Equal(t, expectedBaseTokenName, swapTokenPair.BasePooledCoin.Denom)
			require.Equal(t, expectedQuoteTokenName, swapTokenPair.QuotePooledCoin.Denom)
		}
	}
}

func initToken(name string) token.Token {
	return token.Token{
		Description:         name,
		Symbol:              name,
		OriginalSymbol:      name,
		WholeName:           name,
		OriginalTotalSupply: sdk.NewDec(0),
		Owner:               supply.NewModuleAddress(ModuleName),
		Type:                1,
		Mintable:            true,
	}
}

func TestHandleMsgAddLiquidity(t *testing.T) {
	mapp, addrKeysSlice := getMockAppWithBalance(t, 1, 100000)
	keeper := mapp.swapKeeper
	mapp.BeginBlock(abci.RequestBeginBlock{Header: abci.Header{Height: 2}})
	ctx := mapp.BaseApp.NewContext(false, abci.Header{}).WithBlockHeight(10).WithBlockTime(time.Now())
	testToken := initToken(types.TestBasePooledToken)
	testQuoteToken := initToken(types.TestQuotePooledToken)

	mapp.supplyKeeper.SetSupply(ctx, supply.NewSupply(mapp.TotalCoinsSupply))
	handler := NewHandler(keeper)
	msg := types.NewMsgCreateExchange(testToken.Symbol, types.TestQuotePooledToken, addrKeysSlice[0].Address)
	mapp.tokenKeeper.NewToken(ctx, testToken)
	mapp.tokenKeeper.NewToken(ctx, testQuoteToken)

	result := handler(ctx, msg)
	require.Equal(t, "", result.Log)

	testQuoteToken2 := initToken(types.TestBasePooledToken2)
	mapp.tokenKeeper.NewToken(ctx, testQuoteToken2)
	msgPool2 := types.NewMsgCreateExchange(testToken.Symbol, types.TestBasePooledToken2, addrKeysSlice[0].Address)
	result2 := handler(ctx, msgPool2)
	require.Equal(t, "", result2.Log)

	minLiquidity := sdk.NewDec(1)
	maxBaseAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(10000))
	quoteAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDec(10000))
	quoteAmount2 := sdk.NewDecCoinFromDec(types.TestBasePooledToken2, sdk.NewDec(10000))
	nonExistMaxBaseAmount := sdk.NewDecCoinFromDec("abc", sdk.NewDec(10000))
	invalidMinLiquidity := sdk.NewDec(1000)
	invalidMaxBaseAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(1))
	insufficientMaxBaseAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(1000000))
	insufficientQuoteAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDec(1000000))
	deadLine := time.Now().Unix()
	addr := addrKeysSlice[0].Address

	tests := []struct {
		testCase         string
		minLiquidity     sdk.Dec
		maxBaseAmount    sdk.DecCoin
		quoteAmount      sdk.DecCoin
		deadLine         int64
		addr             sdk.AccAddress
		exceptResultCode sdk.CodeType
	}{
		{"success", minLiquidity, maxBaseAmount, quoteAmount, deadLine, addr, 0},
		{"success(not native token)", minLiquidity, maxBaseAmount, quoteAmount2, deadLine, addr, 0},
		{"blockTime exceeded deadline", minLiquidity, maxBaseAmount, quoteAmount, 0, addr, sdk.CodeInternal},
		{"unknown swapTokenPair", minLiquidity, nonExistMaxBaseAmount, quoteAmount, deadLine, addr, sdk.CodeInternal},
		{"The required baseTokens are greater than MaxBaseAmount", minLiquidity, invalidMaxBaseAmount, quoteAmount, deadLine, addr, sdk.CodeInternal},
		{"The available liquidity is less than MinLiquidity", invalidMinLiquidity, maxBaseAmount, quoteAmount, deadLine, addr, sdk.CodeInternal},
		{"insufficient Coins", minLiquidity, insufficientMaxBaseAmount, insufficientQuoteAmount, deadLine, addr, sdk.CodeInsufficientCoins},
	}

	for _, testCase := range tests {
		addLiquidityMsg := types.NewMsgAddLiquidity(testCase.minLiquidity, testCase.maxBaseAmount, testCase.quoteAmount, testCase.deadLine, testCase.addr)
		result = handler(ctx, addLiquidityMsg)
		require.Equal(t, testCase.exceptResultCode, result.Code)
	}

	acc := mapp.AccountKeeper.GetAccount(ctx, addr)
	require.False(t, acc.GetCoins().Empty())
	queryCheck := make(map[string]sdk.Dec)
	var err error
	testPoolToken := types.GetPoolTokenName(types.TestBasePooledToken, types.TestQuotePooledToken)
	queryCheck[testPoolToken], err = sdk.NewDecFromStr("1")
	testPoolToken2 := types.GetPoolTokenName(types.TestBasePooledToken, types.TestBasePooledToken2)
	queryCheck[testPoolToken2], err = sdk.NewDecFromStr("1")
	require.Nil(t, err)
	queryCheck[types.TestQuotePooledToken] = sdk.NewDec(90000)
	queryCheck[types.TestBasePooledToken] = sdk.NewDec(80000)
	queryCheck[types.TestBasePooledToken2] = sdk.NewDec(90000)
	queryCheck[types.TestBasePooledToken3] = sdk.NewDec(100000)

	for _, c := range acc.GetCoins() {
		fmt.Println(c)
		value, ok := queryCheck[c.Denom]
		require.True(t, ok)
		require.Equal(t, value, c.Amount)
	}
}

func TestHandleMsgRemoveLiquidity(t *testing.T) {
	mapp, addrKeysSlice := getMockAppWithBalance(t, 1, 100000)
	keeper := mapp.swapKeeper
	mapp.BeginBlock(abci.RequestBeginBlock{Header: abci.Header{Height: 2}})
	ctx := mapp.BaseApp.NewContext(false, abci.Header{}).WithBlockHeight(10).WithBlockTime(time.Now())
	testToken := initToken(types.TestBasePooledToken)
	testQuoteToken := initToken(types.TestQuotePooledToken)

	mapp.supplyKeeper.SetSupply(ctx, supply.NewSupply(mapp.TotalCoinsSupply))
	handler := NewHandler(keeper)
	msg := types.NewMsgCreateExchange(testToken.Symbol, types.TestQuotePooledToken, addrKeysSlice[0].Address)
	mapp.tokenKeeper.NewToken(ctx, testToken)
	mapp.tokenKeeper.NewToken(ctx, testQuoteToken)

	result := handler(ctx, msg)
	require.Equal(t, "", result.Log)

	testQuoteToken2 := initToken(types.TestBasePooledToken2)
	mapp.tokenKeeper.NewToken(ctx, testQuoteToken2)
	msgPool2 := types.NewMsgCreateExchange(testToken.Symbol, types.TestBasePooledToken2, addrKeysSlice[0].Address)
	result2 := handler(ctx, msgPool2)
	require.Equal(t, "", result2.Log)

	minLiquidity := sdk.NewDec(1)
	maxBaseAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(10000))
	quoteAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDec(10000))
	deadLine := time.Now().Unix()
	addr := addrKeysSlice[0].Address

	addLiquidityMsg := types.NewMsgAddLiquidity(minLiquidity, maxBaseAmount, quoteAmount, deadLine, addr)
	result = handler(ctx, addLiquidityMsg)
	require.Equal(t, "", result.Log)

	quoteAmount2 := sdk.NewDecCoinFromDec(types.TestBasePooledToken2, sdk.NewDec(10000))
	addLiquidityMsg2 := types.NewMsgAddLiquidity(minLiquidity, maxBaseAmount, quoteAmount2, deadLine, addr)
	result = handler(ctx, addLiquidityMsg2)
	require.Equal(t, "", result.Log)

	liquidity, err := sdk.NewDecFromStr("0.01")
	require.Nil(t, err)
	minBaseAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(1))
	minQuoteAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDec(1))
	minQuoteAmount2 := sdk.NewDecCoinFromDec(types.TestBasePooledToken2, sdk.NewDec(1))
	nonExistMinBaseAmount := sdk.NewDecCoinFromDec("abc", sdk.NewDec(10000))
	invalidMinBaseAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(1000000))
	invalidMinQuoteAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDec(1000000))
	invalidLiquidity := sdk.NewDec(1)

	tests := []struct {
		testCase         string
		liquidity        sdk.Dec
		minBaseAmount    sdk.DecCoin
		minQuoteAmount   sdk.DecCoin
		deadLine         int64
		addr             sdk.AccAddress
		exceptResultCode sdk.CodeType
	}{
		{"success", liquidity, minBaseAmount, minQuoteAmount, deadLine, addr, sdk.CodeOK},
		{"success(not native token)", liquidity, minBaseAmount, minQuoteAmount2, deadLine, addr, sdk.CodeOK},
		{"blockTime exceeded deadline", liquidity, minBaseAmount, minQuoteAmount, 0, addr, sdk.CodeInternal},
		{"unknown swapTokenPair", liquidity, nonExistMinBaseAmount, minQuoteAmount, deadLine, addr, sdk.CodeInternal},
		{"The available baseAmount are less than MinBaseAmount", liquidity, invalidMinBaseAmount, minQuoteAmount, deadLine, addr, sdk.CodeInternal},
		{"The available quoteAmount are less than MinQuoteAmount", liquidity, minBaseAmount, invalidMinQuoteAmount, deadLine, addr, sdk.CodeInternal},
		{"insufficient poolToken", invalidLiquidity, minBaseAmount, minQuoteAmount, deadLine, addr, sdk.CodeInsufficientCoins},
	}

	for _, testCase := range tests {
		fmt.Println(testCase.testCase)
		addLiquidityMsg := types.NewMsgRemoveLiquidity(testCase.liquidity, testCase.minBaseAmount, testCase.minQuoteAmount, testCase.deadLine, testCase.addr)
		result = handler(ctx, addLiquidityMsg)
		require.Equal(t, testCase.exceptResultCode, result.Code)
	}

	acc := mapp.AccountKeeper.GetAccount(ctx, addr)
	require.False(t, acc.GetCoins().Empty())
	queryCheck := make(map[string]sdk.Dec)
	testPoolToken := types.GetPoolTokenName(types.TestBasePooledToken, types.TestQuotePooledToken)
	queryCheck[testPoolToken], err = sdk.NewDecFromStr("0.99")
	testPoolToken2 := types.GetPoolTokenName(types.TestBasePooledToken, types.TestBasePooledToken2)
	queryCheck[testPoolToken2], err = sdk.NewDecFromStr("0.99")
	require.Nil(t, err)
	queryCheck[types.TestQuotePooledToken] = sdk.NewDec(90100)
	queryCheck[types.TestBasePooledToken] = sdk.NewDec(80200)
	queryCheck[types.TestBasePooledToken2] = sdk.NewDec(90100)
	queryCheck[types.TestBasePooledToken3] = sdk.NewDec(100000)

	for _, c := range acc.GetCoins() {
		value, ok := queryCheck[c.Denom]
		require.True(t, ok)
		require.Equal(t, value, c.Amount)
	}
}

func TestHandleMsgTokenToTokenExchange(t *testing.T) {
	mapp, addrKeysSlice := getMockAppWithBalance(t, 1, 100000)
	keeper := mapp.swapKeeper
	mapp.BeginBlock(abci.RequestBeginBlock{Header: abci.Header{Height: 2}})
	ctx := mapp.BaseApp.NewContext(false, abci.Header{}).WithBlockHeight(10).WithBlockTime(time.Now())
	testToken := initToken(types.TestBasePooledToken)
	secondTestTokenName := types.TestBasePooledToken2
	secondTestToken := initToken(secondTestTokenName)
	testQuoteToken := initToken(types.TestQuotePooledToken)
	mapp.swapKeeper.SetParams(ctx, types.DefaultParams())

	mapp.supplyKeeper.SetSupply(ctx, supply.NewSupply(mapp.TotalCoinsSupply))
	handler := NewHandler(keeper)
	msgCreateExchange := types.NewMsgCreateExchange(testToken.Symbol, types.TestQuotePooledToken, addrKeysSlice[0].Address)
	msgCreateExchange2 := types.NewMsgCreateExchange(secondTestToken.Symbol, types.TestQuotePooledToken, addrKeysSlice[0].Address)
	mapp.tokenKeeper.NewToken(ctx, testToken)
	mapp.tokenKeeper.NewToken(ctx, secondTestToken)
	mapp.tokenKeeper.NewToken(ctx, testQuoteToken)

	result := handler(ctx, msgCreateExchange)
	require.Equal(t, "", result.Log)
	result = handler(ctx, msgCreateExchange2)
	require.Equal(t, "", result.Log)

	minLiquidity := sdk.NewDec(1)
	maxBaseAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(10000))
	maxBaseAmount2 := sdk.NewDecCoinFromDec(secondTestTokenName, sdk.NewDec(10000))
	quoteAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDec(10000))
	deadLine := time.Now().Unix()
	addr := addrKeysSlice[0].Address

	addLiquidityMsg := types.NewMsgAddLiquidity(minLiquidity, maxBaseAmount, quoteAmount, deadLine, addr)
	result = handler(ctx, addLiquidityMsg)
	require.Equal(t, "", result.Log)
	addLiquidityMsg2 := types.NewMsgAddLiquidity(minLiquidity, maxBaseAmount2, quoteAmount, deadLine, addr)
	result = handler(ctx, addLiquidityMsg)
	require.Equal(t, "", result.Log)
	result = handler(ctx, addLiquidityMsg2)
	require.Equal(t, "", result.Log)

	minBoughtTokenAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(1))
	deadLine = time.Now().Unix()
	soldTokenAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDec(2))
	insufficientSoldTokenAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDec(100000000))
	unkownBoughtTokenAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken3, sdk.NewDec(1))
	invalidMinBoughtTokenAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(100000))

	minBoughtTokenAmount2 := sdk.NewDecCoinFromDec(secondTestTokenName, sdk.NewDec(1))
	unkownBountTokenAmount2 := sdk.NewDecCoinFromDec(types.TestBasePooledToken3, sdk.NewDec(1))
	soldTokenAmount2 := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(2))
	unkownSoldTokenAmount2 := sdk.NewDecCoinFromDec(types.TestBasePooledToken3, sdk.NewDec(1))
	insufficientSoldTokenAmount2 := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(10000000))
	invalidMinBoughtTokenAmount2 := sdk.NewDecCoinFromDec(secondTestTokenName, sdk.NewDec(100000))
	tests := []struct {
		testCase             string
		minBoughtTokenAmount sdk.DecCoin
		soldTokenAmount      sdk.DecCoin
		deadLine             int64
		recipient            sdk.AccAddress
		addr                 sdk.AccAddress
		exceptResultCode     sdk.CodeType
	}{
		{"(tokenToNativeToken) success", minBoughtTokenAmount, soldTokenAmount, deadLine, addr, addr, 0},
		{"(tokenToToken) success", minBoughtTokenAmount2, soldTokenAmount2, deadLine, addr, addr, 0},
		{"(tokenToNativeToken) blockTime exceeded deadline", minBoughtTokenAmount, soldTokenAmount, 0, addr, addr, sdk.CodeInternal},
		{"(tokenToToken) blockTime exceeded deadline", minBoughtTokenAmount2, soldTokenAmount2, 0, addr, addr, sdk.CodeInternal},
		{"(tokenToNativeToken) insufficient SoldTokenAmount", minBoughtTokenAmount, insufficientSoldTokenAmount, deadLine, addr, addr, sdk.CodeInsufficientCoins},
		{"(tokenToToken) insufficient SoldTokenAmount", minBoughtTokenAmount2, insufficientSoldTokenAmount2, deadLine, addr, addr, sdk.CodeInsufficientCoins},
		{"(tokenToNativeToken) unknown swapTokenPair", unkownBoughtTokenAmount, soldTokenAmount, deadLine, addr, addr, sdk.CodeInternal},
		{"(tokenToToken) unknown swapTokenPair", unkownBountTokenAmount2, soldTokenAmount2, deadLine, addr, addr, sdk.CodeInternal},
		{"(tokenToToken) unknown swapTokenPair2", minBoughtTokenAmount2, unkownSoldTokenAmount2, deadLine, addr, addr, sdk.CodeInternal},
		{"(tokenToNativeToken) The available BoughtTokenAmount are less than minBoughtTokenAmount", invalidMinBoughtTokenAmount, soldTokenAmount, deadLine, addr, addr, sdk.CodeInternal},
		{"(tokenToToken) The available BoughtTokenAmount are less than minBoughtTokenAmount", invalidMinBoughtTokenAmount2, soldTokenAmount2, deadLine, addr, addr, sdk.CodeInternal},
	}

	for _, testCase := range tests {
		fmt.Println(testCase.testCase)
		addLiquidityMsg := types.NewMsgTokenToToken(testCase.soldTokenAmount, testCase.minBoughtTokenAmount, testCase.deadLine, testCase.recipient, testCase.addr)
		result = handler(ctx, addLiquidityMsg)
		fmt.Println(result.Log)
		require.Equal(t, testCase.exceptResultCode, result.Code)

	}

	acc := mapp.AccountKeeper.GetAccount(ctx, addr)
	require.False(t, acc.GetCoins().Empty())
	queryCheck := make(map[string]sdk.Dec)
	var err error
	testPoolToken1 := types.GetPoolTokenName(types.TestBasePooledToken, types.TestQuotePooledToken)
	queryCheck[testPoolToken1], err = sdk.NewDecFromStr("2")
	require.Nil(t, err)
	testPoolToken2 := types.GetPoolTokenName(types.TestBasePooledToken2, types.TestQuotePooledToken)
	queryCheck[testPoolToken2], err = sdk.NewDecFromStr("1")
	require.Nil(t, err)
	queryCheck[types.TestQuotePooledToken] = sdk.NewDec(69998)
	queryCheck[types.TestBasePooledToken], err = sdk.NewDecFromStr("79999.99380121")
	require.Nil(t, err)
	queryCheck[types.TestBasePooledToken2], err = sdk.NewDecFromStr("90001.98782155")
	require.Nil(t, err)
	queryCheck[types.TestBasePooledToken3] = sdk.NewDec(100000)

	for _, c := range acc.GetCoins() {
		value, ok := queryCheck[c.Denom]
		require.True(t, ok)
		require.Equal(t, value, c.Amount)
	}
}

func TestHandleMsgTokenToTokenDirectly(t *testing.T) {
	mapp, addrKeysSlice := getMockAppWithBalance(t, 1, 100000)
	keeper := mapp.swapKeeper
	mapp.BeginBlock(abci.RequestBeginBlock{Header: abci.Header{Height: 2}})
	ctx := mapp.BaseApp.NewContext(false, abci.Header{}).WithBlockHeight(10).WithBlockTime(time.Now())
	testToken := initToken(types.TestBasePooledToken)
	secondTestToken := initToken(types.TestBasePooledToken2)
	mapp.swapKeeper.SetParams(ctx, types.DefaultParams())

	mapp.supplyKeeper.SetSupply(ctx, supply.NewSupply(mapp.TotalCoinsSupply))
	handler := NewHandler(keeper)
	msgCreateExchange := types.NewMsgCreateExchange(testToken.Symbol, secondTestToken.Symbol, addrKeysSlice[0].Address)
	mapp.tokenKeeper.NewToken(ctx, testToken)
	mapp.tokenKeeper.NewToken(ctx, secondTestToken)

	result := handler(ctx, msgCreateExchange)
	require.Equal(t, "", result.Log)

	minLiquidity := sdk.NewDec(1)
	maxBaseAmount := sdk.NewDecCoinFromDec(testToken.Symbol, sdk.NewDec(10000))
	quoteAmount := sdk.NewDecCoinFromDec(secondTestToken.Symbol, sdk.NewDec(10000))
	deadLine := time.Now().Unix()
	addr := addrKeysSlice[0].Address

	addLiquidityMsg := types.NewMsgAddLiquidity(minLiquidity, maxBaseAmount, quoteAmount, deadLine, addr)
	result = handler(ctx, addLiquidityMsg)
	require.Equal(t, "", result.Log)

	minBoughtTokenAmount := sdk.NewDecCoinFromDec(testToken.Symbol, sdk.NewDec(1))
	deadLine = time.Now().Unix()
	soldTokenAmount := sdk.NewDecCoinFromDec(secondTestToken.Symbol, sdk.NewDec(2))

	tests := []struct {
		testCase             string
		minBoughtTokenAmount sdk.DecCoin
		soldTokenAmount      sdk.DecCoin
		deadLine             int64
		recipient            sdk.AccAddress
		addr                 sdk.AccAddress
		exceptResultCode     sdk.CodeType
	}{
		{
			testCase:             "(tokenToTokenDirectly) success",
			minBoughtTokenAmount: minBoughtTokenAmount,
			soldTokenAmount:      soldTokenAmount,
			deadLine:             deadLine,
			recipient:            addr,
			addr:                 addr,
			exceptResultCode:     sdk.CodeOK},
	}

	for _, testCase := range tests {
		fmt.Println(testCase.testCase)
		addLiquidityMsg := types.NewMsgTokenToToken(testCase.soldTokenAmount, testCase.minBoughtTokenAmount, testCase.deadLine, testCase.recipient, testCase.addr)
		result = handler(ctx, addLiquidityMsg)
		fmt.Println(result.Log)
		require.Equal(t, testCase.exceptResultCode, result.Code)

	}

	acc := mapp.AccountKeeper.GetAccount(ctx, addr)
	require.False(t, acc.GetCoins().Empty())
	queryCheck := make(map[string]sdk.Dec)
	var err error
	testPoolToken1 := types.GetPoolTokenName(testToken.Symbol, secondTestToken.Symbol)
	queryCheck[testPoolToken1], err = sdk.NewDecFromStr("1")
	require.Nil(t, err)

	queryCheck[types.TestBasePooledToken], err = sdk.NewDecFromStr("90001.99360247")
	require.Nil(t, err)
	queryCheck[types.TestBasePooledToken2] = sdk.NewDec(89998)
	require.Nil(t, err)
	queryCheck[types.TestBasePooledToken3] = sdk.NewDec(100000)
	queryCheck[types.TestQuotePooledToken] = sdk.NewDec(100000)

	for _, c := range acc.GetCoins() {
		fmt.Println()
		value, ok := queryCheck[c.Denom]
		require.True(t, ok)
		require.Equal(t, value, c.Amount)
	}
}

func TestGetInputPrice(t *testing.T) {
	defaultFeeRate := sdk.NewDecWithPrec(3, 3)
	inputAmount := sdk.NewDecWithPrec(0, 8)
	inputReserve := sdk.NewDec(100)
	outputReserve := sdk.NewDec(100)
	res := keeper.GetInputPrice(inputAmount, inputReserve, outputReserve, defaultFeeRate)
	require.Equal(t, inputAmount, res)
}

func TestRandomData(t *testing.T) {
	mapp, addrKeysSlice := getMockAppWithBalance(t, 1, 100000000)
	keeper := mapp.swapKeeper
	mapp.BeginBlock(abci.RequestBeginBlock{Header: abci.Header{Height: 2}})
	ctx := mapp.BaseApp.NewContext(false, abci.Header{}).WithBlockHeight(10).WithBlockTime(time.Now())
	mapp.swapKeeper.SetParams(ctx, types.DefaultParams())
	testToken := initToken(types.TestBasePooledToken)
	testQuoteToken := initToken(types.TestQuotePooledToken)

	mapp.supplyKeeper.SetSupply(ctx, supply.NewSupply(mapp.TotalCoinsSupply))
	handler := NewHandler(keeper)
	mapp.tokenKeeper.NewToken(ctx, testToken)
	mapp.tokenKeeper.NewToken(ctx, testQuoteToken)
	msgCreateExchange := types.NewMsgCreateExchange(testToken.Symbol, types.TestQuotePooledToken, addrKeysSlice[0].Address)
	result := handler(ctx, msgCreateExchange)
	require.Equal(t, "", result.Log)
	addr := addrKeysSlice[0].Address
	result = handler(ctx, buildRandomMsgAddLiquidity(addr))
	require.True(t, result.Code.IsOK())

	for i := 0; i < 100; i++ {
		var msg sdk.Msg
		judge := rand.Intn(3)
		switch judge {
		case 0:
			msg = buildRandomMsgAddLiquidity(addr)
		case 1:
			msg = buildRandomMsgRemoveLiquidity(addr)
		case 2:
			msg = buildRandomMsgTokenToToken(addr)
		}
		res := handler(ctx, msg)
		if !res.Code.IsOK() {
			fmt.Println(mapp.tokenKeeper.GetCoins(ctx, addr))
			swapTokenPair, err := mapp.swapKeeper.GetSwapTokenPair(ctx, types.TestSwapTokenPairName)
			require.Nil(t, err)
			fmt.Println(swapTokenPair)
			fmt.Println("poolToken: " + keeper.GetPoolTokenAmount(ctx, swapTokenPair.PoolTokenName).String())
			fmt.Println(res.Log)
		}
	}

}

func buildRandomMsgAddLiquidity(addr sdk.AccAddress) types.MsgAddLiquidity {
	minLiquidity := sdk.NewDec(0)
	d := rand.Intn(100) + 1
	d2 := rand.Intn(100) + 1
	maxBaseAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDecWithPrec(int64(d), 8))
	quoteAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDecWithPrec(int64(d2), 8))
	deadLine := time.Now().Unix()
	msg := types.NewMsgAddLiquidity(minLiquidity, maxBaseAmount, quoteAmount, deadLine, addr)
	return msg
}

func buildRandomMsgRemoveLiquidity(addr sdk.AccAddress) types.MsgRemoveLiquidity {
	liquidity := sdk.NewDec(1)
	minBaseAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDecWithPrec(1, 8))
	minQuoteAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDecWithPrec(1, 8))
	deadLine := time.Now().Unix()
	msg := types.NewMsgRemoveLiquidity(liquidity, minBaseAmount, minQuoteAmount, deadLine, addr)
	return msg
}

func buildRandomMsgTokenToToken(addr sdk.AccAddress) types.MsgTokenToToken {
	minBoughtTokenAmount := sdk.NewDecCoinFromDec(types.TestBasePooledToken, sdk.NewDec(0))
	d := rand.Intn(100) + 1
	soldTokenAmount := sdk.NewDecCoinFromDec(types.TestQuotePooledToken, sdk.NewDecWithPrec(int64(d), 8))
	deadLine := time.Now().Unix()
	judge := rand.Intn(2)
	var msg types.MsgTokenToToken
	if judge == 0 {
		msg = types.NewMsgTokenToToken(soldTokenAmount, minBoughtTokenAmount, deadLine, addr, addr)
	} else {
		msg = types.NewMsgTokenToToken(minBoughtTokenAmount, soldTokenAmount, deadLine, addr, addr)
	}

	return msg
}
