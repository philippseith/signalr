package signalr

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"io/ioutil"
	"net/http"
	"net/url"
)

// NewHTTPClient creates a signalR Client using the websocket transport
func NewHTTPClient(ctx context.Context, address string, options ...func(party) error) (Client, error) {
	req, err := http.NewRequest("POST", fmt.Sprintf("%v/negotiate", address), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%v -> %v", req, resp.Status)
	}
	body, _ := ioutil.ReadAll(resp.Body)
	nr := negotiateResponse{}
	err = json.Unmarshal(body, &nr)
	if err != nil {
		return nil, err
	}
	reqUrl, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	q := reqUrl.Query()
	q.Set("id", nr.ConnectionID)
	reqUrl.RawQuery = q.Encode()
	// Select the best connection
	var conn Connection
	if formats := nr.getTransferFormats("WebTransports"); formats != nil {
		// TODO
	} else if formats := nr.getTransferFormats("WebSockets"); formats != nil {
		wsUrl := reqUrl
		wsUrl.Scheme = "ws"
		ws, err := websocket.Dial(wsUrl.String(), "", "http://localhost")
		if err != nil {
			return nil, err
		}
		conn = newWebSocketConnection(ctx, context.Background(), nr.ConnectionID, ws)
	} else if formats := nr.getTransferFormats("ServerSentEvents"); formats != nil {
		req, err := http.NewRequest("GET", reqUrl.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "text/event-stream")
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		conn, err = newClientSSEConnection(ctx, address, nr.ConnectionID, resp.Body)
		if err != nil {
			return nil, err
		}
	}
	if conn != nil {
		result, err := NewClient(ctx, conn, options...)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, nil
}
