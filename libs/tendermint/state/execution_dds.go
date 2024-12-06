package state

import (
	"fmt"
	"github.com/okex/exchain/libs/system/trace"
	"sync/atomic"
	"time"

	"github.com/okex/exchain/libs/iavl"
	"github.com/okex/exchain/libs/system"
	"github.com/okex/exchain/libs/tendermint/delta"
	persist_delta "github.com/okex/exchain/libs/tendermint/delta/persist-delta"
	redis_cgi "github.com/okex/exchain/libs/tendermint/delta/redis-cgi"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	"github.com/spf13/viper"

	"github.com/okex/exchain/libs/tendermint/types"
)

type identityMapType map[string]int64

func (m identityMapType) String() string {
	var output string
	var comma string
	for k, v := range m {
		output += fmt.Sprintf("%s%s=%d", comma, k, v)
		comma = ","
	}
	return output
}

func (m identityMapType) increase(from string, num int64) {
	if len(from) > 0 {
		m[from] += num
	}
}

var EmptyGenerateWatchDataF = func() ([]byte, error) { return nil, nil }

type WatchDataManager interface {
	CreateWatchDataGenerator() func() ([]byte, error)
	UnmarshalWatchData([]byte) (interface{}, error)
	ApplyWatchData(interface{})
}

type EmptyWatchDataManager struct{}

func (e EmptyWatchDataManager) CreateWatchDataGenerator() func() ([]byte, error) {
	return EmptyGenerateWatchDataF
}
func (e EmptyWatchDataManager) UnmarshalWatchData([]byte) (interface{}, error) { return nil, nil }
func (e EmptyWatchDataManager) ApplyWatchData(interface{})                     {}

var (
	evmWatchDataManager  WatchDataManager = EmptyWatchDataManager{}
	wasmWatchDataManager WatchDataManager = EmptyWatchDataManager{}
)

func SetEvmWatchDataManager(manager WatchDataManager) {
	evmWatchDataManager = manager
}

func SetWasmWatchDataManager(manager WatchDataManager) {
	wasmWatchDataManager = manager
}

type DeltaContext struct {
	deltaReader       delta.DeltaReader
	deltaWriter       delta.DeltaWriter
	lastFetchedHeight int64
	dataMap           *deltaMap

	downloadDelta bool
	uploadDelta   bool
	hit           float64
	missed        float64
	logger        log.Logger
	compressType  int
	compressFlag  int
	bufferSize    int

	idMap    identityMapType
	identity string
}

func newDeltaContext(l log.Logger) *DeltaContext {
	dp := &DeltaContext{
		dataMap:       newDataMap(),
		missed:        1,
		downloadDelta: types.DownloadDelta,
		uploadDelta:   types.UploadDelta,
		idMap:         make(identityMapType),
		logger:        l,
	}

	if dp.uploadDelta && dp.downloadDelta {
		panic("download delta is not allowed if upload delta enabled")
	}

	if dp.uploadDelta {
		dp.compressType = viper.GetInt(types.FlagDDSCompressType)
		dp.compressFlag = viper.GetInt(types.FlagDDSCompressFlag)
		dp.setIdentity()
	}
	return dp
}

func (dc *DeltaContext) init() {
	// TODO: how to get buffer size & redis expire from cmdline
	// if only using delta-mode & delta-service-url?
	dc.bufferSize = viper.GetInt(types.FlagBufferSize)
	if dc.bufferSize < 5 {
		dc.bufferSize = 5
	}
	expire := time.Duration(viper.GetInt(types.FlagRedisExpire)) * time.Second
	var err error

	if types.IsDeltaModeUp() {
		dc.deltaWriter, err = redis_cgi.NewRedisClientWithURL(types.DeltaServceURL(), expire, dc.logger)
		if err != nil {
			panic(err)
		}
	} else if types.IsDeltaModeDownRedis() {
		dc.deltaReader, err = redis_cgi.NewRedisClientWithURL(types.DeltaServceURL(), expire, dc.logger)
		if err != nil {
			panic(err)
		}
	} else if types.IsDeltaModeDownPersist() {
		dc.deltaReader = persist_delta.NewPersistDeltaClient(types.DeltaServceURL(), dc.logger)
	} else if dc.uploadDelta || dc.downloadDelta {
		dc.bufferSize = viper.GetInt(types.FlagBufferSize)
		if dc.bufferSize < 5 {
			dc.bufferSize = 5
		}
		url := viper.GetString(types.FlagRedisUrl)
		auth := viper.GetString(types.FlagRedisAuth)
		expire := time.Duration(viper.GetInt(types.FlagRedisExpire)) * time.Second
		dbNum := viper.GetInt(types.FlagRedisDB)
		if dbNum < 0 || dbNum > 15 {
			panic("delta-redis-db only support 0~15")
		}
		rclient := redis_cgi.NewRedisClient(url, auth, expire, dbNum, dc.logger)
		if dc.uploadDelta {
			dc.deltaWriter = rclient
		}
		if dc.downloadDelta {
			dc.deltaReader = rclient
		}
		dc.logger.Info("Init delta broker", "url", url)
	}

	// control if iavl produce delta or not
	iavl.SetProduceDelta(dc.uploadDelta)

	if dc.downloadDelta {
		go dc.downloadRoutine()
	}

	dc.logger.Info("DeltaContext init",
		"uploadDelta", dc.uploadDelta,
		"downloadDelta", dc.downloadDelta,
		"buffer-size", dc.bufferSize,
	)

}

func (dc *DeltaContext) setIdentity() {

	var err error
	dc.identity, err = system.GetIpAddr(viper.GetBool(types.FlagAppendPid))

	if err != nil {
		dc.logger.Error("Failed to set identity", "err", err)
		return
	}

	dc.logger.Info("Set identity", "identity", dc.identity)
}

func (dc *DeltaContext) hitRatio() float64 {
	return dc.hit / (dc.hit + dc.missed)
}

func (dc *DeltaContext) statistic(applied bool, txnum int, delta *DeltaInfo) {
	if applied {
		dc.hit += float64(txnum)
		dc.idMap.increase(delta.from, int64(txnum))
	} else {
		dc.missed += float64(txnum)
	}
}

func (dc *DeltaContext) postApplyBlock(height int64, deltaInfo *DeltaInfo,
	abciResponses *ABCIResponses, deltaMap interface{}, isFastSync bool) {

	// delta consumer
	if dc.downloadDelta {

		applied := false
		deltaLen := 0
		if deltaInfo != nil {
			applied = true
			deltaLen = deltaInfo.deltaLen
		}

		dc.statistic(applied, len(abciResponses.DeliverTxs), deltaInfo)

		trace.GetElapsedInfo().AddInfo(trace.Delta,
			fmt.Sprintf("applied<%t>, ratio<%.2f>, from<%s>",
				applied, dc.hitRatio(), dc.idMap))

		dc.logger.Info("Post apply block", "height", height, "delta-applied", applied,
			"applied-ratio", dc.hitRatio(), "delta-length", deltaLen)

		if applied && types.FastQuery {
			evmWatchDataManager.ApplyWatchData(deltaInfo.watchData)
			wasmWatchDataManager.ApplyWatchData(deltaInfo.wasmWatchData)
		}
	}

	// delta producer
	if dc.uploadDelta && !types.WasmStoreCode {
		trace.GetElapsedInfo().AddInfo(trace.Delta, fmt.Sprintf("ratio<%.2f>", dc.hitRatio()))

		wdFunc := evmWatchDataManager.CreateWatchDataGenerator()
		wasmWdFunc := wasmWatchDataManager.CreateWatchDataGenerator()
		go dc.uploadData(height, abciResponses, deltaMap, wdFunc, wasmWdFunc)
	}
	types.WasmStoreCode = false
}

func (dc *DeltaContext) uploadData(height int64, abciResponses *ABCIResponses, deltaMap interface{}, wdFunc, wasmWdFunc func() ([]byte, error)) {
	if abciResponses == nil || deltaMap == nil {
		dc.logger.Error("Failed to upload", "height", height, "error", fmt.Errorf("empty data"))
		return
	}

	delta4Upload := &types.Deltas{
		Height:       height,
		CompressType: dc.compressType,
		CompressFlag: dc.compressFlag,
		From:         dc.identity,
	}

	var err error
	info := DeltaInfo{abciResponses: abciResponses, treeDeltaMap: deltaMap, marshalWatchData: wdFunc, wasmMarshalWatchData: wasmWdFunc}
	delta4Upload.Payload, err = info.dataInfo2Bytes()
	if err != nil {
		dc.logger.Error("Failed convert dataInfo2Bytes", "target-height", height, "error", err)
	}

	dc.uploadRoutine(delta4Upload, float64(len(abciResponses.DeliverTxs)))
}

func (dc *DeltaContext) uploadRoutine(deltas *types.Deltas, txnum float64) {
	if deltas == nil {
		return
	}
	dc.missed += txnum
	locked := dc.deltaWriter.GetLocker()
	dc.logger.Info("Try to upload delta:", "target-height", deltas.Height, "locked", locked, "delta", deltas)

	if !locked {
		return
	}

	defer dc.deltaWriter.ReleaseLocker()

	upload := func(mrh int64) bool {
		return dc.upload(deltas, txnum, mrh)
	}
	reset, mrh, err := dc.deltaWriter.ResetMostRecentHeightAfterUpload(deltas.Height, upload)
	if !reset {
		dc.logger.Info("Failed to reset mrh:",
			"target-height", deltas.Height,
			"existing-mrh", mrh,
			"err", err)
	}
}

func (dc *DeltaContext) upload(deltas *types.Deltas, txnum float64, mrh int64) bool {
	if deltas == nil {
		dc.logger.Error("Failed to upload nil delta")
		return false
	}

	if deltas.Size() == 0 {
		dc.logger.Error("Failed to upload empty delta",
			"target-height", deltas.Height,
			"mrh", mrh)
		return false
	}

	// marshal deltas to bytes
	deltaBytes, err := deltas.Marshal()
	if err != nil {
		dc.logger.Error("Failed to upload delta",
			"target-height", deltas.Height,
			"mrh", mrh,
			"error", err)
		return false
	}

	t2 := time.Now()
	// set into dds
	if err = dc.deltaWriter.SetDeltas(deltas.Height, deltaBytes); err != nil {
		dc.logger.Error("Failed to upload delta", "target-height", deltas.Height,
			"mrh", mrh, "error", err)
		return false

	}
	t3 := time.Now()
	dc.missed -= txnum
	dc.hit += txnum
	dc.logger.Info("Uploaded delta successfully",
		"target-height", deltas.Height,
		"mrh", mrh,
		"marshal", deltas.MarshalOrUnmarshalElapsed(),
		"calcHash", deltas.HashElapsed(),
		"compress", deltas.CompressOrUncompressElapsed(),
		"upload", t3.Sub(t2),
		"missed", dc.missed,
		"uploaded", dc.hit,
		"deltas", deltas)
	return true
}

// get delta from dds
func (dc *DeltaContext) prepareStateDelta(height int64) *DeltaInfo {
	if !dc.downloadDelta {
		return nil
	}

	deltaInfo, mrh := dc.dataMap.fetch(height)

	atomic.StoreInt64(&dc.lastFetchedHeight, height)

	var succeed bool
	if deltaInfo != nil {
		if deltaInfo.deltaHeight != height {
			dc.logger.Error("Prepared an invalid delta!!!",
				"expected-height", height,
				"mrh", mrh,
				"delta-height", deltaInfo.deltaHeight)
			return nil
		}
		succeed = true
	}

	dc.logger.Info("Prepare delta", "expected-height", height,
		"mrh", mrh, "succeed", succeed)
	return deltaInfo
}

type downloadInfo struct {
	lastTarget            int64
	firstErrorMap         map[int64]error
	lastErrorMap          map[int64]error
	mrhWhen1stErrHappens  map[int64]int64
	mrhWhenlastErrHappens map[int64]int64
	retried               map[int64]int64
	logger                log.Logger
}

func (dc *DeltaContext) downloadRoutine() {
	var targetHeight int64
	var lastRemoved int64
	buffer := int64(dc.bufferSize)
	info := &downloadInfo{
		firstErrorMap:         make(map[int64]error),
		lastErrorMap:          make(map[int64]error),
		mrhWhen1stErrHappens:  make(map[int64]int64),
		mrhWhenlastErrHappens: make(map[int64]int64),
		retried:               make(map[int64]int64),
		logger:                dc.logger,
	}

	ticker := time.NewTicker(50 * time.Millisecond)

	for range ticker.C {
		lastFetchedHeight := atomic.LoadInt64(&dc.lastFetchedHeight)
		if targetHeight <= lastFetchedHeight {
			// move ahead to lastFetchedHeight + 1
			targetHeight = lastFetchedHeight + 1

			// git rid of all deltas before <targetHeight>
			removed, left := dc.dataMap.remove(lastFetchedHeight)
			dc.logger.Info("Reset target height",
				"target-height", targetHeight,
				"last-fetched", lastFetchedHeight,
				"removed", removed,
				"left", left,
			)
		} else {
			if targetHeight%10 == 0 && lastRemoved != lastFetchedHeight {
				removed, left := dc.dataMap.remove(lastFetchedHeight)
				dc.logger.Info("Remove stale deltas",
					"target-height", targetHeight,
					"last-fetched", lastFetchedHeight,
					"removed", removed,
					"left", left,
				)
				lastRemoved = lastFetchedHeight
			}
		}

		lastFetchedHeight = atomic.LoadInt64(&dc.lastFetchedHeight)
		if targetHeight > lastFetchedHeight+buffer {
			continue
		}

		err, delta, mrh := dc.download(targetHeight)
		info.statistics(targetHeight, err, mrh)
		if err != nil {
			continue
		}

		// unmarshal delta bytes to delta info
		deltaInfo := &DeltaInfo{
			from:        delta.From,
			deltaLen:    delta.Size(),
			deltaHeight: delta.Height,
		}
		err = deltaInfo.bytes2DeltaInfo(&delta.Payload)
		if err == nil {
			dc.dataMap.insert(targetHeight, deltaInfo, mrh)
			targetHeight++
		}
	}
}

func (info *downloadInfo) clear(height int64) {
	delete(info.firstErrorMap, height)
	delete(info.lastErrorMap, height)
	delete(info.retried, height)
	delete(info.mrhWhenlastErrHappens, height)
	delete(info.mrhWhen1stErrHappens, height)
}

func (info *downloadInfo) dump(msg string, target int64) {
	info.logger.Info(msg,
		"target-height", target,
		"retried", info.retried[target],
		"1st-err", info.firstErrorMap[target],
		"mrh-when-1st-err", info.mrhWhen1stErrHappens[target],
		"last-err", info.lastErrorMap[target],
		"mrh-when-last-err", info.mrhWhenlastErrHappens[target],
		"map-size", len(info.retried),
	)
	info.clear(target)
}

func (info *downloadInfo) statistics(height int64, err error, mrh int64) {
	if err != nil {
		if _, ok := info.firstErrorMap[height]; !ok {
			info.firstErrorMap[height] = err
			info.mrhWhen1stErrHappens[height] = mrh
		}
		info.lastErrorMap[height] = err
		info.retried[height]++
		info.mrhWhenlastErrHappens[height] = mrh
	} else {
		info.dump("Download info", height)
	}

	if info.lastTarget != height {
		if _, ok := info.retried[info.lastTarget]; ok {
			info.dump("Failed to download", info.lastTarget)
		}
		info.lastTarget = height
	}
}

func (dc *DeltaContext) download(height int64) (error, *types.Deltas, int64) {
	dc.logger.Debug("Download delta started:", "target-height", height)

	t0 := time.Now()
	deltaBytes, err, latestHeight := dc.deltaReader.GetDeltas(height)
	if err != nil {
		return err, nil, latestHeight
	}
	t1 := time.Now()

	// unmarshal
	delta := &types.Deltas{}

	err = delta.Unmarshal(deltaBytes)
	if err != nil {
		dc.logger.Error("Downloaded an invalid delta:", "target-height", height, "err", err)
		return err, nil, latestHeight
	}

	cacheMap, cacheList := dc.dataMap.info()
	dc.logger.Info("Downloaded delta successfully:",
		"target-height", height,
		"cacheMap", cacheMap,
		"cacheList", cacheList,
		"download", t1.Sub(t0),
		"calcHash", delta.HashElapsed(),
		"uncompress", delta.CompressOrUncompressElapsed(),
		"unmarshal", delta.MarshalOrUnmarshalElapsed(),
		"delta", delta)

	return nil, delta, latestHeight
}
