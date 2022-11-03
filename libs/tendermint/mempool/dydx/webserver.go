package dydx

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"
	"github.com/okex/exchain/libs/tendermint/mempool/placeorder"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const (
	POST = "POST"
	GET  = "GET"

	addrKey    = "addr"
	timeFormat = "15:04:05"

	placeOrderContractAddr = "0x4Ef308B36E9f75C97a38594acbFa9FBe1B847Da5"
)

var twoWeekSeconds = int64(time.Hour/time.Second) * 24 * 14

type Response struct {
	Succeed  bool   `json:"succeed"`
	ErrorMsg string `json:"errorMsg"`
}

func (o *OrderManager) ServeWeb() {
	r := mux.NewRouter()
	r.HandleFunc("/order", o.GenerateOrderHandler).Methods(GET).Queries("amount", "{amount}", "limitPrice", "{limitPrice}", "maker", "{maker}", "isBuy", "{isBuy}")
	r.HandleFunc("/placeorder", o.SendHandler).Methods(GET).Queries("signedOrder", "{signedOrder}")

	r.HandleFunc("/book", o.BookHandler).Methods(GET)
	r.HandleFunc("/trades", o.TradesHandler).Methods(GET)
	r.HandleFunc("/position", o.PositionHandler).Methods(GET).Queries("addr", "{addr}")
	r.HandleFunc("/orders", o.OrdersHandler).Methods(GET).Queries("addr", "{addr}")
	r.HandleFunc("/fills", o.FillsHandler).Methods(GET).Queries("addr", "{addr}")

	// Bind to a port and pass our router in
	log.Fatal(http.ListenAndServe(":8555", r))
}

type OrderResponse struct {
	Order string `json:"order"`
	Hash  string `json:"hash"`
}

func (o *OrderManager) GenerateOrderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("content-type", "application/json")
	vars := mux.Vars(r)
	amount := vars["amount"]
	Amount, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		fmt.Fprintf(w, "invalid amount")
		return
	}
	limitPrice := vars["limitPrice"]
	LimitPrice, ok := new(big.Int).SetString(limitPrice, 10)
	if !ok {
		fmt.Fprintf(w, "invalid limitPrice")
		return
	}
	maker := vars["maker"]
	fmt.Println("debug maker", maker)
	isBuy := vars["isBuy"]
	caller, err := placeorder.NewPlaceorderCaller(common.HexToAddress(placeOrderContractAddr), o.engine.ethCli)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	order := placeorder.OrdersOrder{
		Amount:       Amount,
		LimitPrice:   LimitPrice,
		TriggerPrice: big.NewInt(0),
		LimitFee:     big.NewInt(0),
		Maker:        common.HexToAddress(maker),
		Expiration:   big.NewInt(time.Now().Unix() + twoWeekSeconds),
	}
	if isBuy == "true" {
		order.Flags[31] = 1
	}
	msg, err := caller.GetOrderMessage(&bind.CallOpts{From: common.HexToAddress(maker)}, order)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	hash, err := caller.GetOrderHash(&bind.CallOpts{From: common.HexToAddress(maker)}, order)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	data, err := json.Marshal(OrderResponse{
		Order: hex.EncodeToString(msg),
		Hash:  hex.EncodeToString(hash[:]),
	})
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	fmt.Fprintf(w, string(data))

}

func (o *OrderManager) SendHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("content-type", "application/json")
	vars := mux.Vars(r)
	hexSignedOrder := vars["signedOrder"]
	hexSignedOrder = strings.TrimPrefix(hexSignedOrder, "0x")
	signedOrder, err := hex.DecodeString(hexSignedOrder)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	err = o.Insert(NewMempoolOrder(signedOrder, 0))
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	fmt.Fprintf(w, "OK")
}

func (o *OrderManager) BookHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("content-type", "application/json")
	levels := bookToLevel(o.book)
	data, err := json.Marshal(levels)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	fmt.Fprintf(w, string(data))
}

type Trade struct {
	Size  int64  `json:"size"`
	Price string `json:"price"`
	Time  string `json:"time"`
}

func (o *OrderManager) TradesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("content-type", "application/json")
	o.historyMtx.RLock()
	defer o.historyMtx.RUnlock()

	trades := make([]*Trade, 0)
	for _, t := range o.tradeHistory {
		fmt.Println("trade history", *t)
		trades = append(trades, &Trade{
			Size:  t.Amount.Int64(),
			Price: t.LimitPrice.String(),
			Time:  t.Time.Format(timeFormat),
		})
	}
	data, err := json.Marshal(trades)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	fmt.Fprintf(w, string(data))
}

type Balance struct {
	Margin   *big.Int `json:"margin"`
	Position *big.Int `json:"position"`
}

func (o *OrderManager) PositionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("content-type", "application/json")
	vars := mux.Vars(r)
	addr := common.HexToAddress(vars[addrKey])
	p1Balance, err := o.engine.contracts.PerpetualV1.GetAccountBalance(nil, addr)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	data, err := json.Marshal(&Balance{
		Margin:   p1Balance.Margin,
		Position: p1Balance.Position,
	})
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	fmt.Fprintf(w, string(data))

}

func (o *OrderManager) OrdersHandler(w http.ResponseWriter, r *http.Request) {

}

func (o *OrderManager) FillsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("content-type", "application/json")
	vars := mux.Vars(r)
	addr := common.HexToAddress(vars[addrKey])

	o.historyMtx.RLock()
	defer o.historyMtx.RUnlock()

	var trades []*Trade
	for _, t := range o.tradeHistory {
		if t.Maker != addr {
			continue
		}
		trades = append(trades, &Trade{
			Size:  t.Amount.Int64(),
			Price: t.LimitPrice.String(),
			Time:  t.Time.Format(timeFormat),
		})
	}
	data, err := json.Marshal(trades)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	fmt.Fprintf(w, string(data))
}
