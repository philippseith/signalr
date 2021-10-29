package mux_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/julienschmidt/httprouter"

	gmux "github.com/gorilla/mux"
	"github.com/philippseith/signalr/mux"

	"github.com/philippseith/signalr"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func initHttpServeMux(server signalr.Server, port int) {
	router := http.NewServeMux()
	server.MapHTTP(mux.WithHttpServeMux(router), "/hub")
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
	}()
}

func initGorillaRouter(server signalr.Server, port int) {
	router := gmux.NewRouter()
	server.MapHTTP(mux.WithGorillaRouter(router), "/hub")
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
	}()
}

func initHttpRouter(server signalr.Server, port int) {
	router := httprouter.New()
	server.MapHTTP(mux.WithHttpRouter(router), "/hub")
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
	}()
}

func initChiRouter(server signalr.Server, port int) {
	router := chi.NewRouter()
	server.MapHTTP(mux.WithChiRouter(router), "/hub")
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), router)
	}()
}

var _ = Describe("WithXXXRouter", func() {
	for i, initFunc := range []func(server signalr.Server, port int){
		initHttpServeMux,
		initGorillaRouter,
		initHttpRouter,
		initChiRouter,
	} {
		routerNames := []string{
			"http.ServeMux",
			"gorilla/mux.Router",
			"julienschmidt/httprouter",
			"chi/Router",
		}
		Context(fmt.Sprintf("With %v", routerNames[i]), func() {
			Context("A correct negotiation request is sent", func() {
				It("should send a correct negotiation response", func() {
					// Start server
					server, err := signalr.NewServer(context.TODO(), signalr.SimpleHubFactory(&addHub{}), signalr.HTTPTransports("WebSockets"))
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
				})
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
