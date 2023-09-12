package signalr_test

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/philippseith/signalr"
)

func TestMain(m *testing.M) {
	npmInstall := exec.Command("npm", "install")
	stdout, err := npmInstall.StdoutPipe()
	if err != nil {
		println(err.Error())
		os.Exit(120)
	}
	stderr, err := npmInstall.StderrPipe()
	if err != nil {
		println(err.Error())
		os.Exit(121)
	}
	if err := npmInstall.Start(); err != nil {
		println(err.Error())
		os.Exit(122)
	}
	outSlurp, _ := io.ReadAll(stdout)
	errSlurp, _ := io.ReadAll(stderr)
	err = npmInstall.Wait()
	if err != nil {
		println(err.Error())
		fmt.Println(string(outSlurp))
		fmt.Println(string(errSlurp))
		os.Exit(123)
	}
	os.Exit(m.Run())
}

func TestServerSmoke(t *testing.T) {
	testServer(t, "^smoke", signalr.HTTPTransports("WebSockets"))
}

func TestServerJsonWebSockets(t *testing.T) {
	testServer(t, "^JSON", signalr.HTTPTransports(signalr.TransportWebSockets))
}

func TestServerJsonSSE(t *testing.T) {
	testServer(t, "^JSON", signalr.HTTPTransports(signalr.TransportServerSentEvents))
}

func TestServerMessagePack(t *testing.T) {
	testServer(t, "^MessagePack", signalr.HTTPTransports(signalr.TransportWebSockets))
}

func testServer(t *testing.T, testNamePattern string, transports func(signalr.Party) error) {
	serverIsUp := make(chan struct{}, 1)
	quitServer := make(chan struct{}, 1)
	serverIsDown := make(chan struct{}, 1)
	go func() {
		runServer(t, serverIsUp, quitServer, transports)
		serverIsDown <- struct{}{}
	}()
	<-serverIsUp
	runJest(t, testNamePattern, quitServer)
	<-serverIsDown
}

func runJest(t *testing.T, testNamePattern string, quitServer chan struct{}) {
	defer func() { quitServer <- struct{}{} }()
	var jest = exec.Command(filepath.FromSlash("node_modules/.bin/jest"), fmt.Sprintf("--testNamePattern=%v", testNamePattern))
	stdout, err := jest.StdoutPipe()
	if err != nil {
		t.Error(err)
	}
	stderr, err := jest.StderrPipe()
	if err != nil {
		t.Error(err)
	}
	if err := jest.Start(); err != nil {
		t.Error(err)
	}
	outSlurp, _ := io.ReadAll(stdout)
	errSlurp, _ := io.ReadAll(stderr)
	err = jest.Wait()
	if err != nil {
		t.Error(err, fmt.Sprintf("\n%s\n%s", outSlurp, errSlurp))
	} else {
		// Strange: Jest reports test results to stderr
		t.Log(fmt.Sprintf("\n%s", errSlurp))
	}
}

func runServer(t *testing.T, serverIsUp chan struct{}, quitServer chan struct{}, transports func(signalr.Party) error) {
	// Install a handler to cancel the server
	doneQuit := make(chan struct{}, 1)
	ctx, cancelSignalRServer := context.WithCancel(context.Background())
	sRServer, _ := signalr.NewServer(ctx, signalr.SimpleHubFactory(&hub{}),
		signalr.KeepAliveInterval(2*time.Second),
		transports,
		testLoggerOption())
	router := http.NewServeMux()
	sRServer.MapHTTP(signalr.WithHTTPServeMux(router), "/hub")

	server := &http.Server{
		Addr:         "127.0.0.1:5001",
		Handler:      router,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	}
	// wait for someone triggering quitServer
	go func() {
		<-quitServer
		// Cancel the signalR server and all its connections
		cancelSignalRServer()
		// Now shutdown the http server
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		// If it does not Shutdown during 10s, try to end it by canceling the context
		defer cancel()
		server.SetKeepAlivesEnabled(false)
		_ = server.Shutdown(ctx)
		doneQuit <- struct{}{}
	}()
	// alternate method for quiting
	router.HandleFunc("/quit", func(http.ResponseWriter, *http.Request) {
		quitServer <- struct{}{}
	})
	t.Logf("Server %v is up", server.Addr)
	serverIsUp <- struct{}{}
	// Run the server
	_ = server.ListenAndServe()
	<-doneQuit
}

type hub struct {
	signalr.Hub
}

func (h *hub) Ping() string {
	return "Pong!"
}

func (h *hub) Touch() {
	h.Clients().Caller().Send("touched")
}

func (h *hub) TriumphantTriple(club string) []string {
	if strings.Contains(club, "FC Bayern") {
		return []string{"German Championship", "DFB Cup", "Champions League"}
	}
	return []string{}
}

type AlcoholicContent struct {
	Drink    string  `json:"drink"`
	Strength float32 `json:"strength"`
}

func (h *hub) AlcoholicContents() []AlcoholicContent {
	return []AlcoholicContent{
		{
			Drink:    "Brunello",
			Strength: 13.5,
		},
		{
			Drink:    "Beer",
			Strength: 4.9,
		},
		{
			Drink:    "Lagavulin Cask Strength",
			Strength: 56.2,
		},
	}
}

func (h *hub) AlcoholicContentMap() map[string]float64 {
	return map[string]float64{
		"Brunello":                13.5,
		"Beer":                    4.9,
		"Lagavulin Cask Strength": 56.2,
	}
}

func (h *hub) LargeCompressableContent() string {
	return strings.Repeat("data_", 10000)
}

func (h *hub) LargeUncompressableContent() string {
	return randString(20000)
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")

func randString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func (h *hub) FiveDates() <-chan string {
	r := make(chan string)
	go func() {
		for i := 0; i < 5; i++ {
			r <- fmt.Sprint(time.Now().Nanosecond())
			time.Sleep(time.Millisecond * 100)
		}
		close(r)
	}()
	return r
}

func (h *hub) UploadStream(upload <-chan int) {
	var allUs []int
	for u := range upload {
		allUs = append(allUs, u)
	}
	h.Clients().Caller().Send("OnUploadComplete", allUs)
}
