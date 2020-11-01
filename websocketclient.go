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

// NewWebsocketClientConnection creates a signalR ClientConnection using the websocket transport
func NewWebsocketClientConnection(ctx context.Context, address string, options ...func(party) error) (ClientConnection, error) {
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
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	wsAddress := fmt.Sprintf("ws://%v%v?id=%v", u.Host, u.Path, nr.ConnectionID)
	ws, err := websocket.Dial(wsAddress, "", address)
	if err != nil {
		return nil, err
	}
	conn := newWebSocketConnection(ctx, context.Background(), nr.ConnectionID, ws)
	result, err := NewClientConnection(ctx, conn, options...)
	if err != nil {
		return nil, err
	}
	return result, nil
}
