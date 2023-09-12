package signalr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"nhooyr.io/websocket"
)

// Doer is the *http.Client interface
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type httpConnection struct {
	client     Doer
	headers    func() http.Header
	transports []TransportType
}

// WithHTTPClient sets the http client used to connect to the signalR server.
// The client is only used for http requests. It is not used for the websocket connection.
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

func WithTransports(transports ...TransportType) func(*httpConnection) error {
	return func(c *httpConnection) error {
		for _, transport := range transports {
			switch transport {
			case TransportWebSockets, TransportServerSentEvents:
				// Supported
			default:
				return fmt.Errorf("unsupported transport %s", transport)
			}
		}
		c.transports = transports
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
		httpConn.client = http.DefaultClient
	}
	if len(httpConn.transports) == 0 {
		httpConn.transports = []TransportType{TransportWebSockets, TransportServerSentEvents}
	}

	reqURL, err := url.Parse(address)
	if err != nil {
		return nil, err
	}

	negotiateURL := *reqURL
	negotiateURL.Path = path.Join(negotiateURL.Path, "negotiate")
	req, err := http.NewRequestWithContext(ctx, "POST", negotiateURL.String(), nil)
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
	defer func() { closeResponseBody(resp.Body) }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%v %v -> %v", req.Method, req.URL.String(), resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	negotiateResponse := negotiateResponse{}
	if err := json.Unmarshal(body, &negotiateResponse); err != nil {
		return nil, err
	}

	q := reqURL.Query()
	q.Set("id", negotiateResponse.ConnectionID)
	reqURL.RawQuery = q.Encode()

	// Select the best connection
	var conn Connection
	switch {
	case negotiateResponse.hasTransport("WebTransports"):
		// TODO

	case httpConn.hasTransport(TransportWebSockets) && negotiateResponse.hasTransport(TransportWebSockets):
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
		} else {
			opts.HTTPHeader = http.Header{}
		}

		for _, cookie := range resp.Cookies() {
			opts.HTTPHeader.Add("Cookie", cookie.String())
		}

		ws, _, err := websocket.Dial(ctx, wsURL.String(), opts)
		if err != nil {
			return nil, err
		}

		// TODO think about if the API should give the possibility to cancel this connection
		conn = newWebSocketConnection(context.Background(), negotiateResponse.ConnectionID, ws)

	case httpConn.hasTransport(TransportServerSentEvents) && negotiateResponse.hasTransport(TransportServerSentEvents):
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

		conn, err = newClientSSEConnection(address, negotiateResponse.ConnectionID, resp.Body)
		if err != nil {
			return nil, err
		}
	}

	return conn, nil
}

// closeResponseBody reads a http response body to the end and closes it
// See https://blog.cubieserver.de/2022/http-connection-reuse-in-go-clients/
// The body needs to be fully read and closed, otherwise the connection will not be reused
func closeResponseBody(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func (h *httpConnection) hasTransport(transport TransportType) bool {
	for _, t := range h.transports {
		if transport == t {
			return true
		}
	}
	return false
}
