package signalr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"nhooyr.io/websocket"
)

// Doer is the *http.Client interface
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type httpConnection struct {
	client  Doer
	headers func() http.Header
}

// WithHTTPClient sets the http client used to connect to the signalR server
func WithHTTPClient(client Doer) func(*httpConnection) error {
	return func(c *httpConnection) error {
		c.client = client
		return nil
	}
}

// WithHTTPHeaders sets the function for providing request headers for HTTP and websocket requests
func WithHTTPHeaders(headers func() http.Header) func(*httpConnection) error {
	return func(c *httpConnection) error {
		c.headers = headers
		return nil
	}
}

// NewHTTPConnection creates a signalR HTTP Connection for usage with a Client.
// ctx can be used to cancel the SignalR negotiation during the creation of the Connection
// but not the Connection itself.
func NewHTTPConnection(ctx context.Context, address string, options ...func(*httpConnection) error) (Connection, error) {
	httpConn := &httpConnection{}

	for _, option := range options {
		if option != nil {
			if err := option(httpConn); err != nil {
				return nil, err
			}
		}
	}

	if httpConn.client == nil {
		httpConn.client = &http.Client{}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/negotiate", address), nil)
	if err != nil {
		return nil, err
	}

	if httpConn.headers != nil {
		req.Header = httpConn.headers()
	}

	resp, err := httpConn.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%v -> %v", req, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	nr := negotiateResponse{}
	if err := json.Unmarshal(body, &nr); err != nil {
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

		// switch to wss for secure connection
		if reqURL.Scheme == "https" {
			wsURL.Scheme = "wss"
		} else {
			wsURL.Scheme = "ws"
		}

		opts := &websocket.DialOptions{}
		if httpConn.headers != nil {
			opts.HTTPHeader = httpConn.headers()
		}

		ws, _, err := websocket.Dial(ctx, wsURL.String(), opts)
		if err != nil {
			return nil, err
		}

		conn = newWebSocketConnection(context.Background(), context.Background(), nr.ConnectionID, ws)
	} else if formats = nr.getTransferFormats("ServerSentEvents"); formats != nil {
		req, err := http.NewRequest("GET", reqURL.String(), nil)
		if err != nil {
			return nil, err
		}

		if httpConn.headers != nil {
			req.Header = httpConn.headers()
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err := httpConn.client.Do(req)
		if err != nil {
			return nil, err
		}

		conn, err = newClientSSEConnection(context.Background(), address, nr.ConnectionID, resp.Body)
		if err != nil {
			return nil, err
		}
	}

	return conn, nil
}
