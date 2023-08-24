package signalr

var TransportWebSockets = "WebSockets"
var TransportServerSentEvents = "ServerSentEvents"

type availableTransport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

type negotiateResponse struct {
	ConnectionToken     string               `json:"connectionToken,omitempty"`
	ConnectionID        string               `json:"connectionId"`
	NegotiateVersion    int                  `json:"negotiateVersion,omitempty"`
	AvailableTransports []availableTransport `json:"availableTransports"`
}

func (nr *negotiateResponse) hasTransport(transportType string) bool {
	for _, transport := range nr.AvailableTransports {
		if transport.Transport == transportType {
			return true
		}
	}
	return false
}
