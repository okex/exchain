package types

import "fmt"

const (
	FlagDeltaMode       = "delta-mode"
	FlagDeltaServiceURL = "delta-service-url"
)

var (
	deltaMode      = DefaultDeltaMode()
	deltaServceURL = ""
)

var (
	deltaModeNull        = "null"
	deltaModeUp          = "up"
	deltaModeDownRedis   = "down-redis"
	deltaModeDownPersist = "down-persist"
)

func DefaultDeltaMode() string {
	return deltaModeNull
}

func AllDeltaModes() []string {
	return []string{
		deltaModeNull,
		deltaModeUp,
		deltaModeDownRedis,
		deltaModeDownPersist,
	}
}

func DeltaServceURL() string {
	return deltaServceURL
}

func InitDeltaFlagInfo(mode, srvURL string) {
	if !isValidMode(mode) {
		panic(fmt.Errorf("invalid delte mode '%s'", mode))
	}

	deltaMode = mode
	deltaServceURL = srvURL

	// set DownloadDelta or UploadDelta if necessary,
	// because there's too many code refer them(especially DownloadDelta) and
	// execute different logic depend their value
	if IsDeltaModeDownload(deltaMode) {
		DownloadDelta = true
	} else if isDeltaModeUp(deltaMode) {
		UploadDelta = true
	}
}

func IsDeltaModeUp() bool {
	return isDeltaModeUp(deltaMode)
}

func IsDeltaModeDownRedis() bool {
	return isDeltaModeDownRedis(deltaMode)
}

func IsDeltaModeDownPersist() bool {
	return isDeltaModeDownPersist(deltaMode)
}

func IsDeltaModeDownload(mode string) bool {
	return isDeltaModeDownRedis(mode) || isDeltaModeDownPersist(mode)
}

func isDeltaModeDownRedis(mode string) bool {
	return mode == deltaModeDownRedis
}

func isDeltaModeDownPersist(mode string) bool {
	return mode == deltaModeDownPersist
}

func isDeltaModeUp(mode string) bool {
	return mode == deltaModeUp
}

func isValidMode(mode string) bool {
	for _, m := range AllDeltaModes() {
		if mode == m {
			return true
		}
	}
	return false
}
