package signalr

import (
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"io/ioutil"
	"net/http"
	"net/url"
)

// NewWebsocketClientConnection creates a signalR ClientConnection using the websocket transport
func NewWebsocketClientConnection(address string) ClientConnection {
	req, err := http.NewRequest("POST", fmt.Sprintf("%v/negotiate", address), nil)
	if err != nil {
		panic(err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := ioutil.ReadAll(resp.Body)
	nr := negotiateResponse{}
	err = json.Unmarshal(body, &nr)
	if err != nil {
		panic(err)
	}
	u, err := url.Parse(address)
	if err != nil {
		panic(err)
	}
	wsAddress := fmt.Sprintf("ws://%v%v?id=%v", u.Host, u.Path, nr.ConnectionID)
	ws, err := websocket.Dial(wsAddress, "", address)
	if err != nil {
		panic(err)
	}
	conn := &webSocketConnection{
		conn:         ws,
		connectionID: nr.ConnectionID,
		timeout:      0,
	}
	result, err := NewClientConnection(conn)
	if err != nil {
		panic(err)
	}
	return result
}
