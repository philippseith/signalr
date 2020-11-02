package signalr

type availableTransport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

type negotiateResponse struct {
	ConnectionID        string               `json:"connectionId"`
	AvailableTransports []availableTransport `json:"availableTransports"`
}

func (nr *negotiateResponse) getTransferFormats(transportType string) []string {
	for _, transport := range nr.AvailableTransports {
		if transport.Transport == transportType {
			return transport.TransferFormats
		}
	}
	return nil
}
