package signalr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/websocket"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"
)

type webSocketHub struct {
	Hub
}

func (w *webSocketHub) Add2(i int) int {
	return i + 2
}

var _ = Describe("Websocket server", func() {

	Context("A correct negotiation request is sent", func() {
		It("should send a correct negotiation response with support for Websockets with text and binary protocol", func() {
			// Start server
			router := http.NewServeMux()
			MapHub(router, "/hub", &webSocketHub{})
			port := freePort()
			go http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			// Negotiate
			negResp := negotiateWebSocketTestServer(port)
			Expect(negResp["connectionId"]).NotTo(BeNil())
			Expect(negResp["availableTransports"]).To(BeAssignableToTypeOf([]interface{}{}))
			avt := negResp["availableTransports"].([]interface{})
			Expect(len(avt)).To(BeNumerically(">", 0))
			Expect(avt[0]).To(BeAssignableToTypeOf(map[string]interface{}{}))
			avtv := avt[0].(map[string]interface{})
			Expect(avtv["transport"]).To(Equal("WebSockets"))
			Expect(avtv["transferFormats"]).To(BeAssignableToTypeOf([]interface{}{}))
			tf := avtv["transferFormats"].([]interface{})
			Expect(tf).To(ContainElement("Text"))
			Expect(tf).To(ContainElement("Binary"))
		})
	})

	Context("A invalid negotiation request is sent", func() {
		It("should send a correct negotiation response with support for Websockets with text and binary protocol", func() {
			// Start server
			router := http.NewServeMux()
			MapHub(router, "/hub", &webSocketHub{})
			port := freePort()
			go http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			// Negotiate the wrong way
			time.Sleep(500)
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%v/hub/negotiateWebSocketTestServer", port))
			Expect(err).To(BeNil())
			Expect(resp).NotTo(BeNil())
			Expect(resp.StatusCode).ToNot(Equal(200))
		})
	})

	Context("When no negotiation is send", func() {
		It("should serve websocket requests without", func() {
			// Start server
			router := http.NewServeMux()
			MapHub(router, "/hub", &webSocketHub{})
			port := freePort()
			go http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			handShakeAndCallWebSocketTestServer(port, "")
		})
	})

	Context("When a negotiation is send", func() {
		It("should serve websocket requests", func() {
			// Start server
			router := http.NewServeMux()
			MapHub(router, "/hub", &webSocketHub{})
			port := freePort()
			go http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			jsonMap := negotiateWebSocketTestServer(port)
			handShakeAndCallWebSocketTestServer(port, fmt.Sprint(jsonMap["connectionId"]))
		})
	})
})

func negotiateWebSocketTestServer(port int) map[string]interface{} {
	time.Sleep(500)
	buf := bytes.Buffer{}
	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%v/hub/negotiateWebSocketTestServer", port), "text/plain;charset=UTF-8", &buf)
	Expect(err).To(BeNil())
	Expect(resp).ToNot(BeNil())
	defer resp.Body.Close()
	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	Expect(err).To(BeNil())
	response := make(map[string]interface{})
	err = json.Unmarshal(body, &response)
	Expect(err).To(BeNil())
	return response
}

func handShakeAndCallWebSocketTestServer(port int, connectionID string) {
	time.Sleep(500)
	logger := log.NewLogfmtLogger(os.Stderr)
	protocol := JSONHubProtocol{}
	protocol.setDebugLogger(level.Debug(logger))
	ws, err := websocket.Dial(fmt.Sprintf("ws://127.0.0.1:%v/hub?id=%v", port, connectionID), "json", "http://127.0.0.1")
	Expect(err).To(BeNil())
	defer ws.Close()
	wsConn := webSocketConnection{ws, connectionID}
	cliConn := newHubConnection(&wsConn, &protocol, level.Info(logger), level.Debug(logger))
	wsConn.Write(append([]byte(`{"protocol": "json","version": 1}`), 30))
	wsConn.Write(append([]byte(`{"type":1,"invocationId":"666","target":"add2","arguments":[1]}`), 30))
	cliConn.Start()
	result := make(chan interface{})
	go func() {
		for {
			if message, err := cliConn.Receive(); err == nil {
				if completionMessage, ok := message.(completionMessage); ok {
					result <- completionMessage.Result
					return
				}
			}
		}
	}()
	select {
	case r := <-result:
		Expect(r).To(Equal(3.0))
	case <-time.After(1000 * time.Millisecond):
		Fail("timed out")
	}
}

func freePort() int {
	if addr, err := net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		if listener, err := net.ListenTCP("tcp", addr); err == nil {
			defer listener.Close()
			return listener.Addr().(*net.TCPAddr).Port
		}
	}
	return 0
}