package types

import (
	"strconv"
	"sync"
)

// Disable followings after milestoneMercuryHeight
// 1. TransferToContractBlock
// 2. ChangeEvmDenomByProposal
// 3. BankTransferBlock
// 4. ibc

var (
	MILESTONE_GENESIS_HEIGHT string
	genesisHeight            int64

	MILESTONE_MERCURY_HEIGHT string
	milestoneMercuryHeight   int64

	MILESTONE_VENUS_HEIGHT string
	milestoneVenusHeight   int64

	MILESTONE_MARS_HEIGHT string
	milestoneMarsHeight   int64

	MILESTONE_VENUS1_HEIGHT string
	milestoneVenus1Height   int64

	MILESTONE_VENUS2_HEIGHT string
	milestoneVenus2Height   int64

	MILESTONE_VENUS3_HEIGHT string
	milestoneVenus3Height   int64

	MILESTONE_EARTH_HEIGHT string
	milestoneEarthHeight   int64

	MILESTONE_VENUS4_HEIGHT string
	milestoneVenus4Height   int64

	MILESTONE_VENUS5_HEIGHT string
	milestoneVenus5Height   int64

	MILESTONE_VENUS6_NAME       = "venus6"
	milestoneVenus6Height int64 = 0

	MILESTONE_VENUS7_NAME       = "venus7"
	milestoneVenus7Height int64 = 0

	MILESTONE_JUPITER_NAME string
	milestoneJupiterHeight int64

	// note: it stores the earlies height of the node,and it is used by cli
	nodePruneHeight int64

	once sync.Once
)

const (
	MainNet = "exchain-66"
	TestNet = "exchain-65"
)

const (
	MainNetVeneus1Height = 12988000
	TestNetVeneus1Height = 12067000

	MainNetVeneusHeight = 8200000
	TestNetVeneusHeight = 8510000

	MainNetMercuyHeight  = 5150000
	TestNetMercuryHeight = 5300000

	MainNetGenesisHeight = 2322600
	TestNetGenesisHeight = 1121818

	TestNetChangeChainId = 2270901
	TestNetChainName1    = "okexchain-65"
)

func init() {
	once.Do(func() {
		genesisHeight = string2number(MILESTONE_GENESIS_HEIGHT)
		milestoneMercuryHeight = string2number(MILESTONE_MERCURY_HEIGHT)
		milestoneVenusHeight = string2number(MILESTONE_VENUS_HEIGHT)
		milestoneMarsHeight = string2number(MILESTONE_MARS_HEIGHT)
		milestoneVenus1Height = string2number(MILESTONE_VENUS1_HEIGHT)
		milestoneVenus2Height = string2number(MILESTONE_VENUS2_HEIGHT)
		milestoneVenus3Height = string2number(MILESTONE_VENUS3_HEIGHT)
		milestoneEarthHeight = string2number(MILESTONE_EARTH_HEIGHT)
		milestoneVenus4Height = string2number(MILESTONE_VENUS4_HEIGHT)
		milestoneVenus5Height = string2number(MILESTONE_VENUS5_HEIGHT)
		milestoneJupiterHeight = string2number(MILESTONE_JUPITER_NAME)
	})
}

func string2number(input string) int64 {
	if len(input) == 0 {
		input = "0"
	}
	res, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		panic(err)
	}
	return res
}

func SetupMainNetEnvironment(pruneH int64) {
	milestoneVenusHeight = MainNetVeneusHeight
	milestoneMercuryHeight = MainNetMercuyHeight
	genesisHeight = MainNetGenesisHeight
	nodePruneHeight = pruneH
	milestoneVenus1Height = MainNetVeneus1Height
}

func SetupTestNetEnvironment(pruneH int64) {
	milestoneVenusHeight = TestNetVeneusHeight
	milestoneMercuryHeight = TestNetMercuryHeight
	genesisHeight = TestNetGenesisHeight
	nodePruneHeight = pruneH
	milestoneVenus1Height = TestNetVeneus1Height
}

// depracate homstead signer support
func HigherThanMercury(height int64) bool {
	if milestoneMercuryHeight == 0 {
		// milestoneMercuryHeight not enabled
		return false
	}
	return height > milestoneMercuryHeight
}

func HigherThanVenus(height int64) bool {
	if milestoneVenusHeight == 0 {
		return false
	}
	return height >= milestoneVenusHeight
}

// use MPT storage model to replace IAVL storage model
func HigherThanMars(height int64) bool {
	if milestoneMarsHeight == 0 {
		return false
	}
	return height > milestoneMarsHeight
}

// change val's pubkey
func HigherThanJupiter(height int64) bool {
	if milestoneJupiterHeight == 0 {
		return false
	}
	return height > milestoneJupiterHeight
}

func GetJupiterHeight() int64 {
	return milestoneJupiterHeight
}

// GetMilestoneVenusHeight returns milestoneVenusHeight
func GetMilestoneVenusHeight() int64 {
	return milestoneVenusHeight
}

// 2322600 is mainnet GenesisHeight
func IsMainNet() bool {
	return MILESTONE_GENESIS_HEIGHT == "2322600"
}

// 1121818 is testnet GenesisHeight
func IsTestNet() bool {
	return MILESTONE_GENESIS_HEIGHT == "1121818"
}

func IsPrivateNet() bool {
	return !IsMainNet() && !IsTestNet()
}

func GetStartBlockHeight() int64 {
	return genesisHeight
}

func GetNodePruneHeight() int64 {
	return nodePruneHeight
}

func GetVenusHeight() int64 {
	return milestoneVenusHeight
}

func GetMercuryHeight() int64 {
	return milestoneMercuryHeight
}

func GetMarsHeight() int64 {
	return milestoneMarsHeight
}

// can be used in unit test only
func UnittestOnlySetMilestoneVenusHeight(height int64) {
	milestoneVenusHeight = height
}

// can be used in unit test only
func UnittestOnlySetMilestoneMarsHeight(height int64) {
	milestoneMarsHeight = height
}

// ==================================
// =========== Venus1 ===============
func HigherThanVenus1(h int64) bool {
	if milestoneVenus1Height == 0 {
		return false
	}
	return h >= milestoneVenus1Height
}

func UnittestOnlySetMilestoneVenus1Height(h int64) {
	milestoneVenus1Height = h
}

func UnittestOnlySetMilestoneVenus6Height(h int64) {
	milestoneVenus6Height = h
}

func GetVenus1Height() int64 {
	return milestoneVenus1Height
}

// =========== Venus1 ===============
// ==================================

// ==================================
// =========== Venus2 ===============
func HigherThanVenus2(h int64) bool {
	if milestoneVenus2Height == 0 {
		return false
	}
	return h >= milestoneVenus2Height
}

func UnittestOnlySetMilestoneVenus2Height(h int64) {
	milestoneVenus2Height = h
}

func GetVenus2Height() int64 {
	return milestoneVenus2Height
}

// =========== Venus2 ===============
// ==================================

// ==================================
// =========== Venus3 ===============
func HigherThanVenus3(h int64) bool {
	if milestoneVenus3Height == 0 {
		return false
	}
	return h > milestoneVenus3Height
}

func UnittestOnlySetMilestoneVenus3Height(h int64) {
	milestoneVenus3Height = h
}

func GetVenus3Height() int64 {
	return milestoneVenus3Height
}

// =========== Venus3 ===============
// ==================================

// ==================================
// =========== Earth ===============
func UnittestOnlySetMilestoneEarthHeight(h int64) {
	milestoneEarthHeight = h
}

func HigherThanEarth(h int64) bool {
	if milestoneEarthHeight == 0 {
		return false
	}
	return h >= milestoneEarthHeight
}

func GetEarthHeight() int64 {
	return milestoneEarthHeight
}

// =========== Earth ===============
// ==================================

// ==================================
// =========== Venus3 ===============
func HigherThanVenus4(h int64) bool {
	if milestoneVenus4Height == 0 {
		return false
	}
	return h > milestoneVenus4Height
}

func UnittestOnlySetMilestoneVenus4Height(h int64) {
	milestoneVenus4Height = h
}

func GetVenus4Height() int64 {
	return milestoneVenus4Height
}

// =========== Venus4 ===============
// ==================================

// ==================================
// =========== Venus5 ===============
func HigherThanVenus5(h int64) bool {
	if milestoneVenus5Height == 0 {
		return false
	}
	return h > milestoneVenus5Height
}

func UnittestOnlySetMilestoneVenus5Height(h int64) {
	milestoneVenus5Height = h
}

func GetVenus5Height() int64 {
	return milestoneVenus5Height
}

// =========== Venus5 ===============
// ==================================

// ==================================
// =========== Venus6 ===============
func HigherThanVenus6(h int64) bool {
	if milestoneVenus6Height == 0 {
		return false
	}
	return h > milestoneVenus6Height
}

func InitMilestoneVenus6Height(h int64) {
	milestoneVenus6Height = h
}

func GetVenus6Height() int64 {
	return milestoneVenus6Height
}

// =========== Venus6 ===============
// ==================================

// ==================================
// =========== Venus7 ===============
func HigherThanVenus7(h int64) bool {
	if milestoneVenus7Height == 0 {
		return false
	}
	return h > milestoneVenus7Height
}

func InitMilestoneVenus7Height(h int64) {
	milestoneVenus7Height = h
}

func GetVenus7Height() int64 {
	return milestoneVenus7Height
}

// =========== Venus7 ===============
// ==================================
