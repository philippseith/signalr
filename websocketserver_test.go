package signalr

import (
	"bytes"
	"context"
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
		// Until SSE and websockets are supported
		XIt("should send a correct negotiation response with support for Websockets with text and binary protocol", func(done Done) {
			// Start server
			server, err := NewServer(context.TODO(), SimpleHubFactory(&webSocketHub{}))
			Expect(err).NotTo(HaveOccurred())
			router := server.MapHub("/hub")
			port := freePort()
			go func() {
				_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			}()
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
			close(done)
		})
	})

	Context("A invalid negotiation request is sent", func() {
		It("should send a correct negotiation response with support for Websockets with text and binary protocol", func(done Done) {
			// Start server
			server, err := NewServer(context.TODO(), SimpleHubFactory(&webSocketHub{}))
			Expect(err).NotTo(HaveOccurred())
			router := server.MapHub("/hub")
			port := freePort()
			go func() {
				_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			}()
			waitForPort(port)
			// Negotiate the wrong way
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%v/hub/negotiate", port))
			Expect(err).To(BeNil())
			Expect(resp).NotTo(BeNil())
			Expect(resp.StatusCode).ToNot(Equal(200))
			close(done)
		})
	})

	Context("When no negotiation is send", func() {
		It("should serve websocket requests", func(done Done) {
			// Start server
			server, err := NewServer(context.TODO(), SimpleHubFactory(&webSocketHub{}))
			Expect(err).NotTo(HaveOccurred())
			router := server.MapHub("/hub")
			port := freePort()
			go func() {
				_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			}()
			waitForPort(port)
			handShakeAndCallWebSocketTestServer(port, "")
			close(done)
		})
	})

	Context("When a negotiation is send", func() {
		It("should serve websocket requests", func(done Done) {
			// Start server
			server, err := NewServer(context.TODO(), SimpleHubFactory(&webSocketHub{}))
			Expect(err).NotTo(HaveOccurred())
			router := server.MapHub("/hub")
			port := freePort()
			go func() {
				_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			}()
			jsonMap := negotiateWebSocketTestServer(port)
			handShakeAndCallWebSocketTestServer(port, fmt.Sprint(jsonMap["connectionId"]))
			close(done)
		})
	})
	Context("Connection with client", func() {
		It("should successfully handle an Invoke call", func(done Done) {
			logger := &nonProtocolLogger{log.NewLogfmtLogger(os.Stderr)}
			// Start server
			server, err := NewServer(context.TODO(),
				SimpleHubFactory(&webSocketHub{}),
				Logger(logger, true))
			Expect(err).NotTo(HaveOccurred())
			router := server.MapHub("/hub")
			port := freePort()
			go func() {
				_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			}()
			waitForPort(port)
			conn, err := NewWebsocketClientConnection(context.TODO(),
				fmt.Sprintf("http://127.0.0.1:%v/hub", port),
				Logger(logger, true))
			Expect(err).NotTo(HaveOccurred())
			Expect(conn).NotTo(BeNil())
			<-conn.Start()
			result := <-conn.Invoke("Add2", 1)
			Expect(result.Error).NotTo(HaveOccurred())
			Expect(result.Value).To(Equal(float64(3)))
			conn2, err := NewWebsocketClientConnection(context.TODO(),
				fmt.Sprintf("http://127.0.0.1:%v/hub", port),
				Logger(logger, true))
			Expect(err).NotTo(HaveOccurred())
			Expect(conn2).NotTo(BeNil())
			<-conn2.Start()
			result = <-conn2.Invoke("Add2", 2)
			Expect(result.Error).NotTo(HaveOccurred())
			Expect(result.Value).To(Equal(float64(4)))
			close(done)
		}, 100)
	})
})

type nonProtocolLogger struct {
	logger StructuredLogger
}

func (n *nonProtocolLogger) Log(keyVals ...interface{}) error {
	for _, kv := range keyVals {
		if kv == "protocol" {
			return nil
		}
	}
	return n.logger.Log(keyVals...)
}

var _ = Describe("Websocket connection", func() {

	Context("The timeout is set with SetTimeout()", func() {
		It("should return the same value in Timeout()", func(done Done) {
			c := &webSocketConnection{}
			c.SetTimeout(time.Millisecond * 100)
			Expect(c.Timeout()).To(Equal(time.Millisecond * 100))
			c.SetTimeout(time.Millisecond * 200)
			Expect(c.Timeout()).To(Equal(time.Millisecond * 200))
			close(done)
		})
	})
})

func negotiateWebSocketTestServer(port int) map[string]interface{} {
	waitForPort(port)
	buf := bytes.Buffer{}
	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%v/hub/negotiate", port), "text/plain;charset=UTF-8", &buf)
	Expect(err).To(BeNil())
	Expect(resp).ToNot(BeNil())
	defer func() {
		_ = resp.Body.Close()
	}()
	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	Expect(err).To(BeNil())
	response := make(map[string]interface{})
	err = json.Unmarshal(body, &response)
	Expect(err).To(BeNil())
	return response
}

func handShakeAndCallWebSocketTestServer(port int, connectionID string) {
	waitForPort(port)
	logger := log.NewLogfmtLogger(os.Stderr)
	protocol := JSONHubProtocol{}
	protocol.setDebugLogger(level.Debug(logger))
	var urlParam string
	if connectionID != "" {
		urlParam = fmt.Sprintf("?id=%v", connectionID)
	}
	ws, err := websocket.Dial(fmt.Sprintf("ws://127.0.0.1:%v/hub%v", port, urlParam), "json", "http://127.0.0.1")
	Expect(err).To(BeNil())
	defer func() {
		_ = ws.Close()
	}()
	wsConn := newWebSocketConnection(connectionID, ws)
	wsConn.initCancel(context.TODO())
	cliConn := newHubConnection(context.TODO(), wsConn, &protocol, 1<<15, log.NewLogfmtLogger(os.Stderr))
	_, _ = wsConn.Write(append([]byte(`{"protocol": "json","version": 1}`), 30))
	_, _ = wsConn.Write(append([]byte(`{"type":1,"invocationId":"666","target":"add2","arguments":[1]}`), 30))
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
			defer func() {
				_ = listener.Close()
			}()
			return listener.Addr().(*net.TCPAddr).Port
		}
	}
	return 0
}

func waitForPort(port int) {
	for {
		if _, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%v", port)); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
