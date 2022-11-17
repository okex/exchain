package dydx

import (
	"container/list"
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	dydxlib "github.com/okex/exchain/libs/dydx"
	"github.com/okex/exchain/libs/dydx/contracts"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/global"
	"github.com/okex/exchain/libs/tendermint/libs/log"
)

type PubSub interface {
	Unsubscribe(id rpc.ID) bool
	SubscribeLogs(conn chan<- *ethtypes.Log, query ethereum.FilterQuery) (rpc.ID, error)
	ParseLogsFromTxs(txResults []*abci.ResponseDeliverTx, query ethereum.FilterQuery) [][]*ethtypes.Log
}

type MatchEngine struct {
	depthBook *DepthBook

	marketPrice *big.Int

	contracts *dydxlib.Contracts
	privKey   *ecdsa.PrivateKey
	from      common.Address
	nonce     uint64
	chainID   *big.Int
	txOps     *bind.TransactOpts

	contractBackend global.LocalEthClient
	httpCli         *ethclient.Client

	config DydxConfig

	pubsub   PubSub
	pubsubID string

	logger log.Logger

	topicLogOrderCanceled common.Hash
	topicLogOrderFilled   common.Hash
	topicLogTrade         common.Hash
	topicLogWithdraw      common.Hash
	topicLogDeposit       common.Hash

	logFilter  ethereum.FilterQuery
	logHandler LogHandler

	frozenOrders *list.List
}

type DydxConfig struct {
	PrivKeyHex    string
	ChainID       string
	EthHttpRpcUrl string

	PerpetualV1ContractAddress string
	P1OrdersContractAddress    string
	P1MakerOracleAddress       string
}

type LogHandler interface {
	HandleOrderFilled(*contracts.P1OrdersLogOrderFilled)
	HandleOrderCanceled(*contracts.P1OrdersLogOrderCanceled)
	HandleTrade(*contracts.PerpetualV1LogTrade)
	HandleWithdraw(withdraw *contracts.PerpetualV1LogWithdraw)
	HandleDeposit(deposit *contracts.PerpetualV1LogDeposit)
}

func NewMatchEngine(api PubSub, accRetriever AccountRetriever, depthBook *DepthBook, config DydxConfig, handler LogHandler, logger log.Logger) (*MatchEngine, error) {
	var engine = &MatchEngine{
		depthBook: depthBook,
		config:    config,
		logger:    logger,
		pubsub:    api,

		frozenOrders:    list.New(),
		contractBackend: global.GetLocalEthClient(),
	}
	if engine.logger == nil {
		engine.logger = log.NewNopLogger()
	}

	var err error
	engine.privKey, err = crypto.HexToECDSA(config.PrivKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	engine.from = crypto.PubkeyToAddress(engine.privKey.PublicKey)
	engine.chainID, _ = new(big.Int).SetString(config.ChainID, 10)
	if engine.chainID == nil {
		return nil, fmt.Errorf("invalid chain id")
	}

	engine.httpCli, _ = ethclient.Dial(config.EthHttpRpcUrl)
	if engine.contractBackend == nil && engine.httpCli != nil {
		engine.contractBackend = engine.httpCli
	}

	engine.txOps, err = bind.NewKeyedTransactorWithChainID(engine.privKey, engine.chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create txOps, err: %w", err)
	}
	ccConfig := &dydxlib.ContractsAddressConfig{
		PerpetualV1:   common.HexToAddress(config.PerpetualV1ContractAddress),
		P1Orders:      common.HexToAddress(config.P1OrdersContractAddress),
		P1MakerOracle: common.HexToAddress(config.P1MakerOracleAddress),
	}
	engine.contracts, err = dydxlib.NewContracts(
		ccConfig,
		engine.txOps,
		engine.contractBackend,
	)

	if handler != nil {
		ordersAbi, err := contracts.P1OrdersMetaData.GetAbi()
		if err != nil {
			return nil, fmt.Errorf("failed to get orders abi, err: %w", err)
		}
		perpetualAbi, err := contracts.PerpetualV1MetaData.GetAbi()
		if err != nil {
			return nil, fmt.Errorf("failed to get perpetual abi, err: %w", err)
		}

		engine.topicLogOrderCanceled = ordersAbi.Events["LogOrderCanceled"].ID
		engine.topicLogOrderFilled = ordersAbi.Events["LogOrderFilled"].ID

		engine.topicLogTrade = perpetualAbi.Events["LogTrade"].ID
		engine.topicLogWithdraw = perpetualAbi.Events["LogWithdraw"].ID
		engine.topicLogDeposit = perpetualAbi.Events["LogDeposit"].ID

		var query = ethereum.FilterQuery{
			Addresses: []common.Address{
				ccConfig.P1Orders,
				ccConfig.PerpetualV1,
			},
			Topics: [][]common.Hash{
				{
					engine.topicLogOrderFilled,
					engine.topicLogOrderCanceled,
					engine.topicLogTrade,
					engine.topicLogWithdraw,
					engine.topicLogDeposit,
				},
			},
		}

		engine.logFilter = query
		engine.logHandler = handler
	}

	if accRetriever != nil {
		engine.nonce = accRetriever.GetAccountNonce(engine.from.String())
	} else {
		engine.nonce, err = engine.contractBackend.NonceAt(context.Background(), engine.from, nil)
		if err != nil {
			return nil, fmt.Errorf("get nonce failed, err: %w", err)
		}
	}
	engine.nonce--
	engine.logger.Info("init operator nonce", "addr", engine.from, "nonce", engine.nonce)

	return engine, nil
}

type MatchResult struct {
	MatchedRecords []*MatchRecord
	TakerOrder     *WrapOrder

	Tx       *ethtypes.Transaction
	tradeOps *dydxlib.TradeOperation
}

func (r *MatchResult) IsEmpty() bool {
	if r == nil {
		return true
	}
	return len(r.MatchedRecords) == 0
}

func (r *MatchResult) Unfreeze() {
	if r == nil {
		return
	}
	for _, record := range r.MatchedRecords {
		if record == nil {
			continue
		}
		record.Maker.Unfrozen(record.Fill.Amount)
		record.Taker.Unfrozen(record.Fill.Amount)
	}
}

func (r *MatchResult) AddMatchedRecord(fill *contracts.P1OrdersFill, makerOrder *WrapOrder) {
	r.MatchedRecords = append(r.MatchedRecords, &MatchRecord{
		Fill:  fill,
		Maker: makerOrder,
		Taker: r.TakerOrder,
	})
}

type MatchRecord struct {
	Fill  *contracts.P1OrdersFill
	Taker *WrapOrder
	Maker *WrapOrder
}

func (record MatchRecord) String() string {
	return fmt.Sprintf("taker %s maker %s,price, %s, amount %s;", record.Taker.Hash(), record.Maker.Hash(), record.Fill.Price, record.Fill.Amount)
}

func (m *MatchEngine) UpdateState(txsResps []*abci.ResponseDeliverTx) {
	if len(txsResps) == 0 {
		return
	}
	logsSlice := m.pubsub.ParseLogsFromTxs(txsResps, m.logFilter)
	for _, logs := range logsSlice {
		for _, evmLog := range logs {
			switch evmLog.Topics[0] {
			case m.topicLogOrderFilled:
				filledLog, err := m.contracts.P1Orders.ParseLogOrderFilled(*evmLog)
				if err == nil {
					m.logHandler.HandleOrderFilled(filledLog)
				}
			case m.topicLogOrderCanceled:
				canceledLog, err := m.contracts.P1Orders.ParseLogOrderCanceled(*evmLog)
				if err == nil {
					m.logHandler.HandleOrderCanceled(canceledLog)
				}
			case m.topicLogTrade:
				tradeLog, err := m.contracts.PerpetualV1.ParseLogTrade(*evmLog)
				if err == nil {
					m.logHandler.HandleTrade(tradeLog)
				}
			case m.topicLogWithdraw:
				withdrawLog, err := m.contracts.PerpetualV1.ParseLogWithdraw(*evmLog)
				if err == nil {
					m.logHandler.HandleWithdraw(withdrawLog)
				}
			case m.topicLogDeposit:
				depositLog, err := m.contracts.PerpetualV1.ParseLogDeposit(*evmLog)
				if err == nil {
					m.logHandler.HandleDeposit(depositLog)
				}
			}
		}
	}
}

func (m *MatchEngine) Match(order *WrapOrder, marketPrice *big.Int) (*MatchResult, error) {
	m.logger.Debug("start match", "order", order, "marketPrice", marketPrice)

	if order.Type() == BuyOrderType {
		return processOrder(order, m.depthBook.sellOrders, m.depthBook, marketPrice)
	} else if order.Type() == SellOrderType {
		return processOrder(order, m.depthBook.buyOrders, m.depthBook, marketPrice)
	} else {
		return nil, fmt.Errorf("invalid order type")
	}
}

func (m *MatchEngine) MatchAndTrade(order *WrapOrder) (*MatchResult, error) {
	marketPrice, err := m.contracts.GetPerpetualV1OraclePrice()
	if err != nil {
		return nil, fmt.Errorf("failed to get market price, err: %w", err)
	}
	matched, err := m.Match(order, marketPrice)
	if err != nil {
		m.logger.Error("failed to match order", "err", err)
		return nil, fmt.Errorf("failed to match order, err: %w", err)
	}

	if matched.IsEmpty() {
		m.logger.Debug("no match", "order", order.Hash())
		return nil, nil
	}

	for i, record := range matched.MatchedRecords {
		m.logger.Debug("match result", "index", i, "record", record)
	}

	var needRollback bool
	defer func() {
		if needRollback {
			m.logger.Debug("match and trade rollback", "order", order.Hash())
			matched.Unfreeze()
		}
	}()

	op := dydxlib.NewTradeOperation(m.contracts)

	for _, record := range matched.MatchedRecords {
		var solOrder1 = WrapOrderToSignedSolOrder(record.Maker)
		var solOrder2 = WrapOrderToSignedSolOrder(record.Taker)
		if record.Maker.Type() == BuyOrderType {
			var tmp = solOrder1
			solOrder1 = solOrder2
			solOrder2 = tmp
		}

		fill := *record.Fill
		fill.Fee = solOrder1.LimitFee
		err = op.FillSignedSolOrderWithTaker(m.from, solOrder1, &fill)
		if err != nil {
			needRollback = true
			return matched, fmt.Errorf("failed to fill order, err: %w", err)
		}
		fill.Fee = solOrder2.LimitFee
		err = op.FillSignedSolOrderWithTaker(m.from, solOrder2, &fill)
		if err != nil {
			needRollback = true
			return matched, fmt.Errorf("failed to fill order, err: %w", err)
		}
	}
	matched.tradeOps = op
	return matched, nil
}

func WrapOrderToSignedSolOrder(order *WrapOrder) *dydxlib.SignedSolOrder {
	return &dydxlib.SignedSolOrder{
		order.P1OrdersOrder, order.Sig,
	}
}

func (m *MatchEngine) trade(order1, order2 *dydxlib.SignedSolOrder, fill *contracts.P1OrdersFill) (*ethtypes.Transaction, error) {
	op := dydxlib.NewTradeOperation(m.contracts)
	err := op.FillSignedSolOrderWithTaker(m.from, order1, fill)
	if err != nil {
		return nil, fmt.Errorf("failed to fill order1, err: %w", err)
	}
	err = op.FillSignedSolOrderWithTaker(m.from, order2, fill)
	if err != nil {
		return nil, fmt.Errorf("failed to fill order2, err: %w", err)
	}
	tx, err := op.Commit(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to commit, err: %w", err)
	}
	return tx, nil
}

func (m *MatchEngine) Rollback(matched *MatchResult) {
	matched.Unfreeze()
	m.depthBook.DeleteByHash(matched.TakerOrder.Hash())
}

func IsIntNilOrZero(i *big.Int) bool {
	return i == nil || i.Cmp(zero) == 0
}

func isValidTriggerPrice(order *WrapOrder, marketPrice *big.Int) bool {
	if !IsIntNilOrZero(order.TriggerPrice) && !IsIntNilOrZero(marketPrice) {
		if order.Type() == BuyOrderType {
			if marketPrice.Cmp(order.TriggerPrice) < 0 {
				return false
			}
		} else {
			if marketPrice.Cmp(order.TriggerPrice) > 0 {
				return false
			}
		}
	}
	return true
}

var zero = big.NewInt(0)

func processOrder(takerOrder *WrapOrder, makerBook *OrderList, book *DepthBook, marketPrice *big.Int) (*MatchResult, error) {
	var matchResult = &MatchResult{
		TakerOrder: takerOrder,
	}
	if takerOrder.LeftAmount.Cmp(zero) <= 0 || !isValidTriggerPrice(takerOrder, marketPrice) {
		err := book.Insert(takerOrder)
		if err != nil {
			return nil, err
		}
		return matchResult, nil
	}

	var makerOrderElem *list.Element

	for {
		if makerOrderElem == nil {
			makerOrderElem = makerBook.Front()
		} else {
			makerOrderElem = makerOrderElem.Next()
		}
		if makerOrderElem == nil {
			break
		}
		makerOrder := makerOrderElem.Value.(*WrapOrder)

		if makerOrder.LeftAmount.Cmp(zero) <= 0 {
			continue
		}

		if takerOrder.Type() == BuyOrderType && takerOrder.Price().Cmp(makerOrder.Price()) < 0 {
			break
		}
		if takerOrder.Type() == SellOrderType && takerOrder.Price().Cmp(makerOrder.Price()) > 0 {
			break
		}

		if !isValidTriggerPrice(makerOrder, marketPrice) {
			continue
		}

		matchPrice := makerOrder.Price()
		if !IsIntNilOrZero(marketPrice) {
			if takerOrder.Type() == BuyOrderType {
				if takerOrder.Price().Cmp(marketPrice) >= 0 && makerOrder.Price().Cmp(marketPrice) <= 0 {
					matchPrice = marketPrice
				}
			} else {
				if makerOrder.Price().Cmp(marketPrice) >= 0 && takerOrder.Price().Cmp(marketPrice) <= 0 {
					matchPrice = marketPrice
				}
			}
		}

		matchAmount := big.NewInt(0).Set(takerOrder.LeftAmount)
		if matchAmount.Cmp(makerOrder.LeftAmount) > 0 {
			matchAmount.Set(makerOrder.LeftAmount)
		}
		matchResult.AddMatchedRecord(&contracts.P1OrdersFill{
			Amount: matchAmount,
			Price:  matchPrice,
		}, makerOrder)

		takerOrder.Frozen(matchAmount)
		makerOrder.Frozen(matchAmount)

		if takerOrder.LeftAmount.Cmp(zero) == 0 {
			break
		}
	}
	err := book.Insert(takerOrder)
	if err != nil {
		return nil, err
	}
	return matchResult, nil
}
