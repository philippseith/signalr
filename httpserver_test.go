package signalr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"nhooyr.io/websocket"
)

type addHub struct {
	Hub
}

func (w *addHub) Add2(i int) int {
	return i + 2
}

func (w *addHub) Echo(s string) string {
	return s
}

var _ = Describe("HTTP server", func() {
	for _, transport := range [][]string{
		{"WebSockets", "Text"},
		{"WebSockets", "Binary"},
		{"ServerSentEvents", "Text"},
	} {
		transport := transport
		Context(fmt.Sprintf("%v %v", transport[0], transport[1]), func() {
			Context("A correct negotiation request is sent", func() {
				It(fmt.Sprintf("should send a correct negotiation response with support for %v with text protocol", transport), func(done Done) {
					// Start server
					server, err := NewServer(context.TODO(), SimpleHubFactory(&addHub{}), HTTPTransports(transport[0]))
					Expect(err).NotTo(HaveOccurred())
					router := http.NewServeMux()
					server.MapHTTP(router, "/hub")
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
					avtVal := avt[0].(map[string]interface{})
					Expect(avtVal["transport"]).To(Equal(transport[0]))
					Expect(avtVal["transferFormats"]).To(BeAssignableToTypeOf([]interface{}{}))
					tf := avtVal["transferFormats"].([]interface{})
					Expect(tf).To(ContainElement("Text"))
					if transport[0] == "WebSockets" {
						Expect(tf).To(ContainElement("Binary"))
					}
					close(done)
				}, 2.0)
			})

			Context("A invalid negotiation request is sent", func() {
				It(fmt.Sprintf("should send a correct negotiation response with support for %v with text protocol", transport), func(done Done) {
					// Start server
					server, err := NewServer(context.TODO(), SimpleHubFactory(&addHub{}), HTTPTransports(transport[0]))
					Expect(err).NotTo(HaveOccurred())
					router := http.NewServeMux()
					server.MapHTTP(router, "/hub")
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
				}, 2.0)
			})

			Context("Connection with client", func() {
				It("should successfully handle an Invoke call", func(done Done) {
					logger := &nonProtocolLogger{log.NewLogfmtLogger(os.Stderr)}
					// Start server
					server, err := NewServer(context.TODO(),
						SimpleHubFactory(&addHub{}), HTTPTransports(transport[0]),
						Logger(logger, true))
					Expect(err).NotTo(HaveOccurred())
					router := http.NewServeMux()
					server.MapHTTP(router, "/hub")
					port := freePort()
					go func() {
						_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
					}()
					waitForPort(port)
					client, err := NewHTTPClient(context.TODO(),
						fmt.Sprintf("http://127.0.0.1:%v/hub", port),
						Logger(logger, true),
						TransferFormat(transport[1]))
					Expect(err).NotTo(HaveOccurred())
					Expect(client).NotTo(BeNil())
					err = client.Start()
					Expect(err).NotTo(HaveOccurred())
					result := <-client.Invoke("Add2", 1)
					Expect(result.Error).NotTo(HaveOccurred())
					Expect(result.Value).To(BeEquivalentTo(3))
					// Try second connection
					client2, err := NewHTTPClient(context.TODO(),
						fmt.Sprintf("http://127.0.0.1:%v/hub", port),
						Logger(logger, true),
						TransferFormat(transport[1]))
					Expect(err).NotTo(HaveOccurred())
					Expect(client2).NotTo(BeNil())
					_ = client2.Start()
					result = <-client2.Invoke("Add2", 2)
					Expect(result.Error).NotTo(HaveOccurred())
					Expect(result.Value).To(BeEquivalentTo(4))
					// Huge message
					hugo := strings.Repeat("#", 2500)
					result = <-client.Invoke("Echo", hugo)
					Expect(result.Error).NotTo(HaveOccurred())
					s := result.Value.(string)
					Expect(s).To(Equal(hugo))
					close(done)
				}, 10.0)
			})
		})
	}
	Context("When no negotiation is send", func() {
		It("should serve websocket requests", func(done Done) {
			// Start server
			server, err := NewServer(context.TODO(), SimpleHubFactory(&addHub{}), HTTPTransports("WebSockets"))
			Expect(err).NotTo(HaveOccurred())
			router := http.NewServeMux()
			server.MapHTTP(router, "/hub")
			port := freePort()
			go func() {
				_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
			}()
			waitForPort(port)
			handShakeAndCallWebSocketTestServer(port, "")
			close(done)
		}, 5.0)
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
	protocol := jsonHubProtocol{}
	protocol.setDebugLogger(level.Debug(logger))
	var urlParam string
	if connectionID != "" {
		urlParam = fmt.Sprintf("?id=%v", connectionID)
	}
	ws, _, err := websocket.Dial(context.Background(), fmt.Sprintf("ws://127.0.0.1:%v/hub%v", port, urlParam), nil)
	Expect(err).To(BeNil())
	defer func() {
		_ = ws.Close(websocket.StatusNormalClosure, "")
	}()
	wsConn := newWebSocketConnection(context.TODO(), context.TODO(), connectionID, ws)
	cliConn := newHubConnection(wsConn, &protocol, 1<<15, log.NewLogfmtLogger(os.Stderr))
	_, _ = wsConn.Write(append([]byte(`{"protocol": "json","version": 1}`), 30))
	_, _ = wsConn.Write(append([]byte(`{"type":1,"invocationId":"666","target":"add2","arguments":[1]}`), 30))
	result := make(chan interface{})
	go func() {
		for recvResult := range cliConn.Receive() {
			if completionMessage, ok := recvResult.message.(completionMessage); ok {
				result <- completionMessage.Result
				return
			}
		}
	}()
	select {
	case r := <-result:
		var f float64
		Expect(protocol.UnmarshalArgument(r, &f)).NotTo(HaveOccurred())
		Expect(f).To(Equal(3.0))
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
