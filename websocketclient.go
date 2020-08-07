package signalr

import (
	//"context"
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"net/url"

	//"golang.org/x/net/websocket"
	"io/ioutil"
	"net/http"
)

func NewWebsocketClientConnection(address string) ClientConnection {
	req, err := http.NewRequest("POST", fmt.Sprintf("%v/negotiate", address), nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	nr := negotiateResponse{}
	err = json.Unmarshal(body, &nr)
	if err != nil {
		panic(err)
	}
	fmt.Println("response %v", nr)
	u, err := url.Parse(address)
	if err != nil {
		panic(err)
	}
	wsAddress := fmt.Sprintf("ws://%v%v?id=%v", u.Host, u.Path, nr.ConnectionID)
	ws, err := websocket.Dial(wsAddress, "", address)
	conn := &webSocketConnection{
		conn:         ws,
		connectionID: nr.ConnectionID,
		timeout:      0,
	}
	result, err := NewClientConnection(conn)
	return result
}
