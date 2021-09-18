package signalr

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"nhooyr.io/websocket"
)

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type httpConnection struct {
	client Doer
}

func HTTPClientOption(client Doer) func(*httpConnection) error {
	return func(c *httpConnection) error {
		c.client = client
		return nil
	}
}

// NewHTTPConnection creates a signalR Client which tries to connect over http to the given address
func NewHTTPConnection(ctx context.Context, address string, options ...func(*httpConnection) error) (Connection, error) {
	httpConn := &httpConnection{}

	for _, option := range options {
		option(httpConn)
	}

	if httpConn.client == nil {
		httpConn.client = &http.Client{}
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%v/negotiate", address), nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpConn.client.Do(req)
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
	var formats []string
	var conn Connection
	if formats = nr.getTransferFormats("WebTransports"); formats != nil {
		// TODO
	} else if formats = nr.getTransferFormats("WebSockets"); formats != nil {
		wsURL := reqURL
		wsURL.Scheme = "ws"
		ws, _, err := websocket.Dial(ctx, wsURL.String(), nil)
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

	return conn, nil
}
