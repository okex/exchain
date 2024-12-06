package delta

type DeltaReader interface {
	GetDeltas(height int64) ([]byte, error, int64)
}

type DeltaWriter interface {
	GetLocker() bool
	ReleaseLocker()
	ResetMostRecentHeightAfterUpload(height int64, upload func(int64) bool) (bool, int64, error)
	SetDeltas(height int64, bytes []byte) error
}
