/*
Package signalr contains a SignalR client and a SignalR server.
Both support the transport types Websockets and Server-Sent Events
and the transfer formats Text (JSON) and Binary (MessagePack).

# Basics

The SignalR Protocol is a protocol for two-way RPC over any stream- or message-based transport.
Either party in the connection may invoke procedures on the other party,
and procedures can return zero or more results or an error.
Typically, SignalR connections are HTTP-based, but it is dead simple to implement a signalr.Connection on any transport
that supports io.Reader and io.Writer.

# Client

A Client can be used in client side code to access server methods. From an existing connection, it can be created with NewClient().

	  // NewClient with raw TCP connection and MessagePack encoding
	  conn, err := net.Dial("tcp", "example.com:6502")
	  client := NewClient(ctx,
				WithConnection(NewNetConnection(ctx, conn)),
				TransferFormat("Binary),
				WithReceiver(receiver))

	  client.Start()

A special case is NewHTTPClient(), which creates a Client from a server address and negotiates with the server
which kind of connection (Websockets, Server-Sent Events) will be used.

	  // Configurable HTTP connection
	  conn, err := NewHTTPConnection(ctx, "http://example.com/hub", WithHTTPHeaders(..))
	  // Client with JSON encoding
	  client, err := NewClient(ctx,
				WithConnection(conn),
				TransferFormat(TransferFormatText),
				WithReceiver(receiver))

	  client.Start()

The object which will receive server callbacks is passed to NewClient() by using the WithReceiver option.
After calling client.Start(), the client is ready to call server methods or to receive callbacks.

# Server

A Server provides the public methods of a server side class over signalr to the client.
Such a server side class is called a hub and must implement HubInterface.
It is reasonable to derive your hubs from the Hub struct type, which already implements HubInterface.
Servers for arbitrary connection types can be created with NewServer().

	// Typical server with log level debug to Stderr
	server, err := NewServer(ctx, SimpleHubFactory(hub), Logger(log.NewLogfmtLogger(os.Stderr), true))

To serve a connection, call server.Serve(connection) in a goroutine. Serve ends when the connection is closed or the
servers context is canceled.

	// Serving over TCP, accepting client who use MessagePack or JSON
	addr, _ := net.ResolveTCPAddr("tcp", "localhost:6502")
	listener, _ := net.ListenTCP("tcp", addr)
	tcpConn, _ := listener.Accept()
	go server.Serve(NewNetConnection(conn))

To serve a HTTP connection, use server.MapHTTP(), which connects the server with a path in an http.ServeMux.
The server then automatically negotiates which kind of connection (Websockets, Server-Sent Events) will be used.

	// build a signalr.Server using your hub
	// and any server options you may need
	server, _ := signalr.NewServer(ctx,
	    signalr.SimpleHubFactory(&AppHub{})
	    signalr.KeepAliveInterval(2*time.Second),
	    signalr.Logger(kitlog.NewLogfmtLogger(os.Stderr), true))
	)

	// create a new http.ServerMux to handle your app's http requests
	router := http.NewServeMux()

	// ask the signalr server to map it's server
	// api routes to your custom baseurl
	server.MapHTTP(signalr.WithHTTPServeMux(router), "/hub")

	// in addition to mapping the signalr routes
	// your mux will need to serve the static files
	// which make up your client-side app, including
	// the signalr javascript files. here is an example
	// of doing that using a local `public` package
	// which was created with the go:embed directive
	//
	// fmt.Printf("Serving static content from the embedded filesystem\n")
	// router.Handle("/", http.FileServer(http.FS(public.FS)))

	// bind your mux to a given address and start handling requests
	fmt.Printf("Listening for websocket connections on http://%s\n", address)
	if err := http.ListenAndServe(address, router); err != nil {
	    log.Fatal("ListenAndServe:", err)
	}

# Supported method signatures

The SignalR protocol constrains the signature of hub or receiver methods that can be used over SignalR.
All methods with serializable types as parameters and return types are supported.
Methods with multiple return values are not generally supported, but returning one or no value and an optional error is supported.

	// Simple signatures for hub/receiver methods
	func (mh *MathHub) Divide(a, b float64) (float64, error) // error on division by zero
	func (ah *AlgoHub) Sort(values []string) []string
	func (ah *AlgoHub) FindKey(value []string, dict map[int][]string) (int, error) // error on not found
	func (receiver *View) DisplayServerValue(value interface{}) // will work for every serializable value

Methods which return a single sending channel (<-chan), and optionally an error, are used to initiate callee side streaming.
The caller will receive the contents of the channel as stream.
When the returned channel is closed, the stream will be completed.

	// Streaming methods
	func (n *Netflix) Stream(show string, season, episode int) (<-chan []byte, error) // error on password shared

Methods with one or multiple receiving channels (chan<-) as parameters are used as receivers for caller side streaming.
The caller invokes this method and pushes one or multiple streams to the callee. The method should end when all channels
are closed. A channel is closed by the server when the assigned stream is completed.
The methods which return a channel are not supported.

	// Caller side streaming
	func (mh *MathHub) MultiplyAndSum(a, b chan<- float64) float64

In most cases, the caller will be the client and the callee the server. But the vice versa case is also possible.
*/
package signalr
