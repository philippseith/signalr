package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-kit/log"

	"github.com/go-chi/chi/v5"

	"github.com/julienschmidt/httprouter"

	"github.com/gorilla/mux"

	"github.com/philippseith/signalr"

	"github.com/philippseith/signalr/router"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRouter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "signalr/mux Suite")
}

func initGorillaRouter(server signalr.Server, port int) {
	r := mux.NewRouter()
	server.MapHTTP(router.WithGorillaRouter(r), "/hub")
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), r)
	}()
}

func initHttpRouter(server signalr.Server, port int) {
	r := httprouter.New()
	server.MapHTTP(router.WithHttpRouter(r), "/hub")
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), r)
	}()
}

func initChiRouter(server signalr.Server, port int) {
	r := chi.NewRouter()
	server.MapHTTP(router.WithChiRouter(r), "/hub")
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), r)
	}()
}

var _ = Describe("Router", func() {
	for i, initFunc := range []func(server signalr.Server, port int){
		initGorillaRouter,
		initHttpRouter,
		initChiRouter,
	} {
		routerNames := []string{
			"gorilla/mux.Router",
			"julienschmidt/httprouter",
			"chi/Router",
		}
		Context(fmt.Sprintf("With %v", routerNames[i]), func() {
			Context("A correct negotiation request is sent", func() {
				It("should send a correct negotiation response", func(done Done) {
					// Start server
					ctx, serverCancel := context.WithCancel(context.Background())
					server, err := signalr.NewServer(ctx, signalr.SimpleHubFactory(&addHub{}), signalr.HTTPTransports(signalr.TransportWebSockets))
					Expect(err).NotTo(HaveOccurred())
					port := freePort()
					initFunc(server, port)
					// Negotiate
					negResp := negotiateWebSocketTestServer(port)
					Expect(negResp["connectionId"]).NotTo(BeNil())
					Expect(negResp["availableTransports"]).To(BeAssignableToTypeOf([]interface{}{}))
					avt := negResp["availableTransports"].([]interface{})
					Expect(len(avt)).To(BeNumerically(">", 0))
					Expect(avt[0]).To(BeAssignableToTypeOf(map[string]interface{}{}))
					avtVal := avt[0].(map[string]interface{})
					Expect(avtVal["transferFormats"]).To(BeAssignableToTypeOf([]interface{}{}))
					tf := avtVal["transferFormats"].([]interface{})
					Expect(tf).To(ContainElement("Text"))
					Expect(tf).To(ContainElement("Binary"))
					serverCancel()
					close(done)
				})
			})
			Context("Connection with client", func() {
				It("should successfully handle an Invoke call", func(done Done) {
					// Start server
					server, err := signalr.NewServer(context.Background(),
						signalr.SimpleHubFactory(&addHub{}),
						signalr.Logger(log.NewNopLogger(), false),
						signalr.HTTPTransports("WebSockets"))
					Expect(err).NotTo(HaveOccurred())
					port := freePort()
					initFunc(server, port)
					waitForPort(port)
					// Start client
					conn, err := signalr.NewHTTPConnection(context.Background(), fmt.Sprintf("http://127.0.0.1:%v/hub", port))
					Expect(err).NotTo(HaveOccurred())
					client, err := signalr.NewClient(context.Background(),
						signalr.WithConnection(conn),
						signalr.Logger(log.NewNopLogger(), false),
						signalr.TransferFormat("Text"))
					Expect(err).NotTo(HaveOccurred())
					Expect(client).NotTo(BeNil())
					errCh := client.Start()
					Expect(client.WaitConnected(context.Background())).NotTo(HaveOccurred())
					go func() {
						defer GinkgoRecover()
						Expect(errors.Is(<-errCh, context.Canceled)).To(BeTrue())
					}()
					Expect(err).NotTo(HaveOccurred())
					result := <-client.Invoke("Add2", 1)
					Expect(result.Error).NotTo(HaveOccurred())
					Expect(result.Value).To(BeEquivalentTo(3))
					close(done)
				}, 10.0)
			})
		})
	}
})

type addHub struct {
	signalr.Hub
}

func (w *addHub) Add2(i int) int {
	return i + 2
}

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
