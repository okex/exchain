package v1

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/okex/exchain/libs/tendermint/delta/internal"
)

const (
	Version = 1

	apiPathGetDelta = "api/v1/delta/"
)

type APIResult struct {
	Success bool   `json:"success"`
	Version int    `json:"version"`
	Data    []byte `json:"data"`
}

type APIErrorResult struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func MakeGetDeltaRequestPath(base string, height int64) string {
	key := internal.GenDeltaKey(height)

	path := base
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	return path + apiPathGetDelta + url.PathEscape(key)
}

func ParseResponse(data []byte, height int64) ([]byte, error, int64) {
	var result APIResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unknown response APIResult returned by persist-delta server"), 0
	}
	if !result.Success {
		var apiError APIErrorResult
		if err := json.Unmarshal(result.Data, &apiError); err != nil {
			return nil, fmt.Errorf("unknown response error data returned by persist-delta server"), 0
		}
		return nil, fmt.Errorf("persist-delta server error: code: %d; message: %s", apiError.Code, apiError.Message), 0
	}

	if result.Version != Version {
		return nil, fmt.Errorf("unexpect response verion: current is %d but got %d", Version, result.Version), 0
	}

	// TODO: most recent height(mrh) seems link useless, delete it.
	return result.Data, nil, height
}
