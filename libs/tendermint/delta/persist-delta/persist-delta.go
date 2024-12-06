package persist_delta

import (
	"io"
	"net/http"

	// using api v1
	api "github.com/okex/exchain/libs/tendermint/delta/persist-delta/internal/v1"

	"github.com/okex/exchain/libs/tendermint/libs/log"
)

type PersistDeltaClient struct {
	url string

	logger log.Logger
}

func NewPersistDeltaClient(srvURL string, logger log.Logger) *PersistDeltaClient {
	return &PersistDeltaClient{
		url: srvURL,

		logger: logger,
	}
}

func (c *PersistDeltaClient) GetDeltas(height int64) ([]byte, error, int64) {
	path := api.MakeGetDeltaRequestPath(c.url, height)

	resp, err := http.Get(path)
	if err != nil {
		c.logger.Error("http get error", path, err)
		return nil, err, 0
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("read http response error", path, err)
		return nil, err, 0
	}

	return api.ParseResponse(data, height)
}
