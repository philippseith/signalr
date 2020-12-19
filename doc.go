/*
package signalr contains a SignalR client and a SignalR server.
Both support the transport types Websockets and Server-Sent Events and the transfer format Text (JSON).
Rudimentary support for binary transfer format (MessagePack) is available.

Basics

The SignalR Protocol is a protocol for two-way RPC over any Message-based transport.
Either party in the connection may invoke procedures on the other party,
and procedures can return zero or more results or an error.
Typically, SignalR connections are HTTP-based, but it is dead simple to implement a signalr.Connection on any transport
that supports io.Reader and io.Writer.

Client

A Client can be used in client side code to access server methods. From an existing connection, it is created with NewClient().
A special case is NewHTTPClient(), which creates a Client from a server address and negotiates with the server
which kind of connection (Websockets, Server-Sent Events) will be used.
The object which will receive server callbacks is passed to NewClient() / NewHTTPClient() by using the Receiver option.
After calling client.Start(), the client is ready to call server methods or to receive callbacks.

Server

A Server provides the public methods of a server side class over signalr to the client.
Such a server side class is called a hub and must implement HubInterface.
It is reasonable to derive your hubs from the Hub struct type, which already implements HubInterface.
To serve a connection, call server.Serve(connection) in a goroutine. Serve ends when the connection is closed or the
servers context is canceled.
To server a HTTP connection, use server.MapHTTP(), which connects the server with a path in an http.ServeMux.
The server then automatically negotiates, which kind of connection (Websockets, Server-Sent Events) will be used.

Supported method signatures

The SignalR protocol constrains the signature of hub or receiver methods that can be used over SignalR.
All methods with serializable types as parameters and return types are supported.
Methods with multiple serializable return values are supported.
Methods which return single sending channel (<-chan) are used to initiate callee side streaming.
The caller will receive the contents of the channel as stream.
When the returned channel is closed, the stream will be completed.
Methods with one or multiple receiving channels (chan<-) as parameters are used as receivers for caller side streaming.
The caller invokes this method and pushes one or multiple streams to the callee. The method should end when all channels
are closed. A channel is closed by the server when the assigned stream is completed.
In most cases, the caller will be the client and the callee the server. But the vice versa case is also possible.
*/
package signalr
