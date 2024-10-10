package types

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/okex/exchain/libs/tendermint/types/time"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateGenesisState(t *testing.T) {
	specs := map[string]struct {
		srcMutator func(*GenesisState)
		expError   bool
	}{
		"all good": {
			srcMutator: func(s *GenesisState) {},
		},
		"params invalid": {
			srcMutator: func(s *GenesisState) {
				s.Params = Params{}
			},
			expError: true,
		},
		"codeinfo invalid": {
			srcMutator: func(s *GenesisState) {
				s.Codes[0].CodeInfo.CodeHash = nil
			},
			expError: true,
		},
		"contract invalid": {
			srcMutator: func(s *GenesisState) {
				s.Contracts[0].ContractAddress = "invalid"
			},
			expError: true,
		},
		"sequence invalid": {
			srcMutator: func(s *GenesisState) {
				s.Sequences[0].IDKey = nil
			},
			expError: true,
		},
		"genesis store code message invalid": {
			srcMutator: func(s *GenesisState) {
				s.GenMsgs[0].GetStoreCode().WASMByteCode = nil
			},
			expError: true,
		},
		"genesis instantiate contract message invalid": {
			srcMutator: func(s *GenesisState) {
				s.GenMsgs[1].GetInstantiateContract().CodeID = 0
			},
			expError: true,
		},
		"genesis execute contract message invalid": {
			srcMutator: func(s *GenesisState) {
				s.GenMsgs[2].GetExecuteContract().Sender = "invalid"
			},
			expError: true,
		},
		"genesis invalid message type": {
			srcMutator: func(s *GenesisState) {
				s.GenMsgs[0].Sum = nil
			},
			expError: true,
		},
	}
	for msg, spec := range specs {
		t.Run(msg, func(t *testing.T) {
			state := GenesisFixture(spec.srcMutator)
			got := state.ValidateBasic()
			if spec.expError {
				require.Error(t, got)
				return
			}
			require.NoError(t, got)
		})
	}
}

func TestCodeValidateBasic(t *testing.T) {
	specs := map[string]struct {
		srcMutator func(*Code)
		expError   bool
	}{
		"all good": {srcMutator: func(_ *Code) {}},
		"code id invalid": {
			srcMutator: func(c *Code) {
				c.CodeID = 0
			},
			expError: true,
		},
		"codeinfo invalid": {
			srcMutator: func(c *Code) {
				c.CodeInfo.CodeHash = nil
			},
			expError: true,
		},
		"codeBytes empty": {
			srcMutator: func(c *Code) {
				c.CodeBytes = []byte{}
			},
			expError: true,
		},
		"codeBytes nil": {
			srcMutator: func(c *Code) {
				c.CodeBytes = nil
			},
			expError: true,
		},
		"codeBytes greater limit": {
			srcMutator: func(c *Code) {
				c.CodeBytes = bytes.Repeat([]byte{0x1}, MaxWasmSize+1)
			},
			expError: true,
		},
	}
	for msg, spec := range specs {
		t.Run(msg, func(t *testing.T) {
			state := CodeFixture(spec.srcMutator)
			got := state.ValidateBasic()
			if spec.expError {
				require.Error(t, got)
				return
			}
			require.NoError(t, got)
		})
	}
}

func TestContractValidateBasic(t *testing.T) {
	specs := map[string]struct {
		srcMutator func(*Contract)
		expError   bool
	}{
		"all good": {srcMutator: func(_ *Contract) {}},
		"contract address invalid": {
			srcMutator: func(c *Contract) {
				c.ContractAddress = "invalid"
			},
			expError: true,
		},
		"contract info invalid": {
			srcMutator: func(c *Contract) {
				c.ContractInfo.Creator = "invalid"
			},
			expError: true,
		},
		"contract with created set": {
			srcMutator: func(c *Contract) {
				c.ContractInfo.Created = &AbsoluteTxPosition{}
			},
			expError: true,
		},
		"contract state invalid": {
			srcMutator: func(c *Contract) {
				c.ContractState = append(c.ContractState, Model{})
			},
			expError: true,
		},
	}
	for msg, spec := range specs {
		t.Run(msg, func(t *testing.T) {
			state := ContractFixture(spec.srcMutator)
			got := state.ValidateBasic()
			if spec.expError {
				require.Error(t, got)
				return
			}
			require.NoError(t, got)
		})
	}
}

//	func TestGenesisContractInfoMarshalUnmarshal(t *testing.T) {
//		var myAddr sdk.WasmAddress = rand.Bytes(ContractAddrLen)
//		var myOtherAddr sdk.WasmAddress = rand.Bytes(ContractAddrLen)
//		anyPos := AbsoluteTxPosition{BlockHeight: 1, TxIndex: 2}
//
//		anyTime := time.Now().UTC()
//		// using gov proposal here as a random protobuf types as it contains an Any type inside for nested unpacking
//		myExtension, err := govtypes.NewProposal(&govtypes.TextProposal{Title: "bar"}, 1, anyTime, anyTime)
//		require.NoError(t, err)
//		myExtension.TotalDeposit = nil
//
//		src := NewContractInfo(1, myAddr, myOtherAddr, "bar", &anyPos)
//		err = src.SetExtension(&myExtension)
//		require.NoError(t, err)
//
//		interfaceRegistry := types.NewInterfaceRegistry()
//		marshaler := codec.NewProtoCodec(interfaceRegistry)
//		RegisterInterfaces(interfaceRegistry)
//		// register proposal as extension type
//		interfaceRegistry.RegisterImplementations(
//			(*ContractInfoExtension)(nil),
//			&govtypes.Proposal{},
//		)
//		// register gov types for nested Anys
//		govtypes.RegisterInterfaces(interfaceRegistry)
//
//		// when encode
//		gs := GenesisState{
//			Contracts: []Contract{{
//				ContractInfo: src,
//			}},
//		}
//
//		bz, err := marshaler.Marshal(&gs)
//		require.NoError(t, err)
//		// and decode
//		var destGs GenesisState
//		err = marshaler.Unmarshal(bz, &destGs)
//		require.NoError(t, err)
//		// then
//		require.Len(t, destGs.Contracts, 1)
//		dest := destGs.Contracts[0].ContractInfo
//		assert.Equal(t, src, dest)
//		// and sanity check nested any
//		var destExt govtypes.Proposal
//		require.NoError(t, dest.ReadExtension(&destExt))
//		assert.Equal(t, destExt.GetTitle(), "bar")
//	}
type Poker struct {
	Poker struct {
		UserHands []string `json:"user_hands"`
		Board     string   `json:"board"`
	} `json:"poker"`
}

func TestPoker(t *testing.T) {
	//Card's Valid ranks: one of [23456789TJQKA]
	//Card's Valid suits: one of [chsd]
	ranks := []string{"2", "3", "4", "5", "6", "7", "8", "9", "T", "J", "Q", "K", "A"}
	suits := []string{"c", "h", "s", "d"}
	ranksLen := len(ranks)
	suitsLen := len(suits)
	number := 5000
	poker := Poker{Poker: struct {
		UserHands []string `json:"user_hands"`
		Board     string   `json:"board"`
	}{
		UserHands: make([]string, 0),
		Board:     "3c 5c As Jc Qh",
	}}
	s := rand.NewSource(time.Now().Unix())
	for i := 0; i < number; i++ {
		randSeed := rand.New(s).Int()

		rankIndex_0 := randSeed % ranksLen
		suitIndex_0 := randSeed % suitsLen
		hands_0 := ranks[rankIndex_0] + suits[suitIndex_0]
		for strings.Contains(poker.Poker.Board, hands_0) {
			randSeed := rand.New(s).Int()
			rankIndex_0 = randSeed % ranksLen
			suitIndex_0 = randSeed % suitsLen
			hands_0 = ranks[rankIndex_0] + suits[suitIndex_0]
		}

		rankIndex_1 := (randSeed + 1) % ranksLen
		suitIndex_1 := (randSeed + 1) % suitsLen
		hands_1 := ranks[rankIndex_1] + suits[suitIndex_1]
		for strings.Contains(poker.Poker.Board+hands_0, hands_1) {
			randSeed := rand.New(s).Int()
			rankIndex_1 = (randSeed + 1) % ranksLen
			suitIndex_1 = (randSeed + 1) % suitsLen
			hands_1 = ranks[rankIndex_1] + suits[suitIndex_1]
		}

		hands := hands_0 + " " + hands_1
		poker.Poker.UserHands = append(poker.Poker.UserHands, hands)
	}
	buff, err := json.Marshal(poker)
	require.NoError(t, err)
	t.Log("poker", string(buff))
	t.Log("hex", hex.EncodeToString(buff))
}

type Poker2 struct {
	Poker2 struct {
		UserHands []string `json:"user_hands"`
		Board     string   `json:"board"`
		Num       int      `json:"num"`
	} `json:"poker_multi"`
}

func TestPoker2(t *testing.T) {
	//Card's Valid ranks: one of [23456789TJQKA]
	//Card's Valid suits: one of [chsd]
	ranks := []string{"2", "3", "4", "5", "6", "7", "8", "9", "T", "J", "Q", "K", "A"}
	suits := []string{"c", "h", "s", "d"}
	ranksLen := len(ranks)
	suitsLen := len(suits)
	number := 5000
	times := 10
	poker := Poker2{Poker2: struct {
		UserHands []string `json:"user_hands"`
		Board     string   `json:"board"`
		Num       int      `json:"num"`
	}{
		UserHands: make([]string, 0),
		Board:     "3c 5c As Jc Qh",
		Num:       times,
	}}
	s := rand.NewSource(time.Now().Unix())
	for i := 0; i < number; i++ {
		randSeed := rand.New(s).Int()

		rankIndex_0 := randSeed % ranksLen
		suitIndex_0 := randSeed % suitsLen
		hands_0 := ranks[rankIndex_0] + suits[suitIndex_0]
		for strings.Contains(poker.Poker2.Board, hands_0) {
			randSeed := rand.New(s).Int()
			rankIndex_0 = randSeed % ranksLen
			suitIndex_0 = randSeed % suitsLen
			hands_0 = ranks[rankIndex_0] + suits[suitIndex_0]
		}

		rankIndex_1 := (randSeed + 1) % ranksLen
		suitIndex_1 := (randSeed + 1) % suitsLen
		hands_1 := ranks[rankIndex_1] + suits[suitIndex_1]
		for strings.Contains(poker.Poker2.Board+hands_0, hands_1) {
			randSeed := rand.New(s).Int()
			rankIndex_1 = (randSeed + 1) % ranksLen
			suitIndex_1 = (randSeed + 1) % suitsLen
			hands_1 = ranks[rankIndex_1] + suits[suitIndex_1]
		}

		hands := hands_0 + " " + hands_1
		poker.Poker2.UserHands = append(poker.Poker2.UserHands, hands)
	}
	buff, err := json.Marshal(poker)
	require.NoError(t, err)
	t.Log("poker", string(buff))
	t.Log("hex", hex.EncodeToString(buff))
}

func TestBase64(t *testing.T) {
	str := "IFOmDgdWAT2B1MmGY9xDyHWi6Kha48B5J2\\/AZpckNkjq0zDPL9PQZeQeVRfhadn98QQNsHaeJEdUIEo0KEiN4wTYWeTrULpt3iUkbf+GV6RRxM8W87HaZEEi3vGV+auABVNWfBpAK9plZN+tCY4m9w=="
	//dbuf, err := base64.StdEncoding.DecodeString(str)
	//require.NoError(t, err)
	//t.Log("dbuf-str:", string(dbuf))
	//t.Log("dbuf-hex:", hex.EncodeToString(dbuf))
	temp := []byte(str)
	t.Log(hex.EncodeToString(temp))
}
