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
