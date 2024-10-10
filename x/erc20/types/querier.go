package types

type ContractByDenomRequest struct {
	Denom string `json:"denom,omitempty"`
}

type DenomByContractRequest struct {
	Contract string `json:"contract,omitempty"`
}

type ContractTemplate struct {
	Proxy     string `json:"proxy"`
	Implement string `json:"implement"`
}

type QueryTokenMappingResponse struct {
	Denom     string `json:"denom"`
	Contract  string `json:"contract"`
	Path      string `json:"path,omitempty"`
	BaseDenom string `json:"base_denom,omitempty"`
}

type TokenMappingByChannelRequest struct {
	Channels  string `json:"channels"`
	BaseDenom string `json:"base_denom"`
}
