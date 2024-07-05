package config

import "time"

type IDynamicConfig interface {
	GetMempoolRecheck() bool
	GetMempoolForceRecheckGap() int64
	GetMempoolSize() int
	GetMempoolCacheSize() int
	GetMaxTxNumPerBlock() int64
	GetEnableDeleteMinGPTx() bool
	GetMaxGasUsedPerBlock() int64
	GetEnablePGU() bool
	GetPGUPercentageThreshold() int64
	GetPGUConcurrency() int
	GetPGUAdjustment() float64
	GetPGUPersist() bool
	GetMempoolFlush() bool
	GetNodeKeyWhitelist() []string
	GetMempoolCheckTxCost() bool
	GetSentryAddrs() []string
	GetCsTimeoutPropose() time.Duration
	GetCsTimeoutProposeDelta() time.Duration
	GetCsTimeoutPrevote() time.Duration
	GetCsTimeoutPrevoteDelta() time.Duration
	GetCsTimeoutPrecommit() time.Duration
	GetCsTimeoutPrecommitDelta() time.Duration
	GetCsTimeoutCommit() time.Duration
	GetEnableWtx() bool
	GetDeliverTxsExecuteMode() int
	GetEnableHasBlockPartMsg() bool
	GetCommitGapOffset() int64
	GetIavlAcNoBatch() bool
	GetDynamicGpWeight() int
	GetDynamicGpCheckBlocks() int
	GetDynamicGpMode() int
	GetDynamicGpMaxTxNum() int64
	GetDynamicGpMaxGasUsed() int64
	GetGasLimitBuffer() uint64
	GetEnableMempoolSimGuFactor() bool
	GetMaxSubscriptionClients() int
	GetPendingPoolBlacklist() string
	GetMaxTxLimitPerPeer() uint64
	GetConsensusIPWhitelist() []string
}

var DynamicConfig IDynamicConfig = MockDynamicConfig{}

func SetDynamicConfig(c IDynamicConfig) {
	DynamicConfig = c
}

type MockDynamicConfig struct {
	enableDeleteMinGPTx    bool
	dynamicGpMode          int
	dynamicGpMaxTxNum      int64
	dynamicGpMaxGasUsed    int64
	maxSubscriptionClients int
}

func (d MockDynamicConfig) GetMempoolRecheck() bool {
	return DefaultMempoolConfig().Recheck
}

func (d MockDynamicConfig) GetMempoolForceRecheckGap() int64 {
	return DefaultMempoolConfig().ForceRecheckGap
}

func (d MockDynamicConfig) GetMempoolSize() int {
	return DefaultMempoolConfig().Size
}

func (d MockDynamicConfig) GetMempoolCacheSize() int {
	return DefaultMempoolConfig().CacheSize
}

func (d MockDynamicConfig) GetMaxTxNumPerBlock() int64 {
	return DefaultMempoolConfig().MaxTxNumPerBlock
}

func (d MockDynamicConfig) GetMaxGasUsedPerBlock() int64 {
	return DefaultMempoolConfig().MaxGasUsedPerBlock
}

func (d MockDynamicConfig) GetEnablePGU() bool {
	return false
}

func (d MockDynamicConfig) GetPGUPercentageThreshold() int64 {
	return 10
}

func (d MockDynamicConfig) GetPGUConcurrency() int {
	return 1
}

func (d MockDynamicConfig) GetPGUAdjustment() float64 {
	return 1
}

func (d MockDynamicConfig) GetPGUPersist() bool {
	return false
}

func (d MockDynamicConfig) GetMempoolFlush() bool {
	return false
}

func (d MockDynamicConfig) GetNodeKeyWhitelist() []string {
	return []string{}
}

func (d MockDynamicConfig) GetMempoolCheckTxCost() bool {
	return false
}

func (d MockDynamicConfig) GetSentryAddrs() []string {
	return []string{}
}

func (d MockDynamicConfig) GetCsTimeoutPropose() time.Duration {
	return DefaultConsensusConfig().TimeoutPropose
}
func (d MockDynamicConfig) GetCsTimeoutProposeDelta() time.Duration {
	return DefaultConsensusConfig().TimeoutProposeDelta
}
func (d MockDynamicConfig) GetCsTimeoutPrevote() time.Duration {
	return DefaultConsensusConfig().TimeoutPrevote
}
func (d MockDynamicConfig) GetCsTimeoutPrevoteDelta() time.Duration {
	return DefaultConsensusConfig().TimeoutPrecommitDelta
}
func (d MockDynamicConfig) GetCsTimeoutPrecommit() time.Duration {
	return DefaultConsensusConfig().TimeoutPrecommit
}
func (d MockDynamicConfig) GetCsTimeoutPrecommitDelta() time.Duration {
	return DefaultConsensusConfig().TimeoutPrecommitDelta
}
func (d MockDynamicConfig) GetCsTimeoutCommit() time.Duration {
	return DefaultConsensusConfig().TimeoutCommit
}

func (d MockDynamicConfig) GetEnableWtx() bool {
	return false
}
func (d MockDynamicConfig) GetDeliverTxsExecuteMode() int {
	return 0
}

func (d MockDynamicConfig) GetEnableHasBlockPartMsg() bool {
	return false
}

func (d MockDynamicConfig) GetEnableDeleteMinGPTx() bool {
	return d.enableDeleteMinGPTx
}

func (d *MockDynamicConfig) SetEnableDeleteMinGPTx(enable bool) {
	d.enableDeleteMinGPTx = enable
}

func (d MockDynamicConfig) GetCommitGapOffset() int64 {
	return 0
}

func (d MockDynamicConfig) GetIavlAcNoBatch() bool {
	return false
}

func (d *MockDynamicConfig) SetDynamicGpMode(value int) {
	if value < 0 || value > 2 {
		return
	}
	d.dynamicGpMode = value
}

func (d MockDynamicConfig) GetDynamicGpMode() int {
	return d.dynamicGpMode
}

func (d MockDynamicConfig) GetDynamicGpCheckBlocks() int {
	return 5
}

func (d MockDynamicConfig) GetDynamicGpWeight() int {
	return 80
}

func (d *MockDynamicConfig) SetDynamicGpMaxTxNum(value int64) {
	if value < 0 {
		return
	}
	d.dynamicGpMaxTxNum = value
}

func (d MockDynamicConfig) GetDynamicGpMaxTxNum() int64 {
	return d.dynamicGpMaxTxNum
}

func (d *MockDynamicConfig) SetDynamicGpMaxGasUsed(value int64) {
	if value < -1 {
		return
	}
	d.dynamicGpMaxGasUsed = value
}

func (d MockDynamicConfig) GetDynamicGpMaxGasUsed() int64 {
	return d.dynamicGpMaxGasUsed
}

func (d MockDynamicConfig) GetGasLimitBuffer() uint64 {
	return 0
}

func (d MockDynamicConfig) GetEnableMempoolSimGuFactor() bool {
	return false
}

func (d MockDynamicConfig) GetMaxSubscriptionClients() int {
	return d.maxSubscriptionClients
}

func (d *MockDynamicConfig) SetMaxSubscriptionClients(value int) {
	if value < 0 {
		return
	}
	d.maxSubscriptionClients = value
}

func (d MockDynamicConfig) GetPendingPoolBlacklist() string {
	return ""
}

func (c MockDynamicConfig) GetMaxTxLimitPerPeer() uint64 {
	return DefaultMempoolConfig().MaxTxLimitPerPeer
}

func (c MockDynamicConfig) GetConsensusIPWhitelist() []string {
	return []string{}
}
