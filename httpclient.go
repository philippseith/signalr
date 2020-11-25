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
func NewHTTPClient(ctx context.Context, address string, options ...func(Party) error) (Client, error) {
	req, err := http.NewRequest("POST", fmt.Sprintf("%v/negotiate", address), nil)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
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
	reqURL, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	q := reqURL.Query()
	q.Set("id", nr.ConnectionID)
	reqURL.RawQuery = q.Encode()
	// Select the best connection
	var conn Connection
	var formats []string
	if formats = nr.getTransferFormats("WebTransports"); formats != nil {
		// TODO
	} else if formats = nr.getTransferFormats("WebSockets"); formats != nil {
		wsURL := reqURL
		wsURL.Scheme = "ws"
		ws, err := websocket.Dial(wsURL.String(), "", "http://localhost")
		if err != nil {
			return nil, err
		}
		conn = newWebSocketConnection(ctx, context.Background(), nr.ConnectionID, ws)
	} else if formats = nr.getTransferFormats("ServerSentEvents"); formats != nil {
		req, err := http.NewRequest("GET", reqURL.String(), nil)
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
		// If only Text is supported, remove possible option for Binary
		var filteredOptions []func(Party) error
		if len(formats) == 1 && formats[0] == "Text" {
			for _, option := range options {
				c := client{}
				_ = option(&c)
				if c.format == "messagepack" {
					continue
				}
				filteredOptions = append(filteredOptions, option)
			}
		} else {
			filteredOptions = options
		}
		result, err := NewClient(ctx, conn, filteredOptions...)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, nil
}
