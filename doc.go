/*
package signalr contains a signalr client and a signalr server.
Both support the signalr transport types Websockets and Server-Sent Events and the transfer format Text (JSON).
Rudimentary support for binary transfer format (MessagePack) is available.
For a deeper understanding of signalr see https://github.com/dotnet/aspnetcore/blob/master/src/SignalR/docs/specs/HubProtocol.md
and https://github.com/dotnet/aspnetcore/blob/master/src/SignalR/docs/specs/TransportProtocols.md

Basics

The SignalR Protocol is a protocol for two-way RPC over any Message-based transport.
Either party in the connection may invoke procedures on the other party,
and procedures can return zero or more results or an error.

Client

A Client can be used in client side code to access server methods. It s created with NewClient(), which gets the connection
to the server. A special case is NewHTTPClient(), which only gets the server address and negotiates with the server
which kind of connection (Websockets, Server-Sent Events) will be used.
The object which will server callbacks is passed to NewClient() / NewHTTPClient() with the Receiver option.
After calling client.Start(), the client is ready to call server methods or to receive callbacks.

Server

A Server provides the public methods of a server side class over signalr to the client.
Such a server side class is called a hub and must implement HubInterface.
It is reasonable to derive your hubs from the Hub struct type, which already implements HubInterface.
To serve a connection, call server.Serve(connection) in a goroutine. Serve ends when the connection is closed or the
servers context is canceled.
To server a HTTP connection, use server.MapHTTP(), which connects the server with a path in an http.ServeMux.
The server then automatically negotiates, which kind of connection (Websockets, Server-Sent Events) will be used.

Supported Hub / Receiver method parameter and return types

All methods with serializable types as parameters and return types are supported.
Methods with multiple return values are supported.
Hub methods with a single sending channel (<-chan) are used to initiate server side streaming.
The client will receive the contents of the channel as stream.
When the returned channel is closed, the stream will be completed.
Hub methods with one or multiple receiving channels (chan<-) as parameters are used as receivers for client side streaming.
The client invokes this method and pushes one or multiple streams to the server. The method should end when all channels
are closed. A channel is closed by the server when the assigned stream is completed.
*/
package signalr
