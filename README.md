# SignalR

[![Actions Status](https://github.com/philippseith/signalr/workflows/Build%20and%20Test/badge.svg)](https://github.com/philippseith/signalr/actions)
[![codecov](https://codecov.io/gh/philippseith/signalr/branch/master/graph/badge.svg)](https://codecov.io/gh/philippseith/signalr)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/philippseith/signalr)](https://pkg.go.dev/github.com/philippseith/signalr)

SignalR is an open-source library that simplifies adding real-time web functionality to apps. 
Real-time web functionality enables server-side code to push content to clients instantly.

Historically it was tied to ASP.NET Core but the 
[protocol](https://github.com/aspnet/AspNetCore/tree/master/src/SignalR/docs/specs) is open and implementable in any language.

This repository contains an implementation of a SignalR server and a SignalR client in go. 
The implementation is based on the work of David Fowler at https://github.com/davidfowl/signalr-ports.
Client and server support transport over WebSockets, Server Sent Events and raw TCP.
Protocol encoding in JSON and MessagePack is fully supported.

- [Install](#install)
- [Getting Started](#getting-started)
    - [Server side](#server-side)
        - [Implement the HubInterface](#implement-the-hubinterface)
        - [Serve with http.ServeMux](#serve-with-httpservemux)
    - [Client side: JavaScript/TypeScript](#client-side-javascripttypescript)
        - [Grab copies of the signalr scripts](#grab-copies-of-the-signalr-scripts)
        - [Use a HubConnection to connect to your server Hub](#use-a-hubconnection-to-connect-to-your-server-hub)
    - [Client side: go](#client-side-go)
- [Debugging](#debugging)

## Install

With a [correctly configured](https://golang.org/doc/install#testing) Go toolchain:

```sh
go get -u github.com/philippseith/signalr
```

## Getting Started

SignalR uses a `signalr.HubInterface` instance to anchor the connection on the server and a javascript `HubConnection` object to anchor the connection on the client.

### Server side

#### Implement the HubInterface

The easiest way to implement the `signalr.HubInterface` in your project is to declare your own type and embed `signalr.Hub` which implements that interface and will take care of all the signalr plumbing. You can call your custom type anything you want so long as it implements the `signalr.HubInterface` interface.

```go
package main

import "github.com/philippseith/signalr"

type AppHub struct {
    signalr.Hub
}
```

Add functions with your custom hub type as a receiver.

```go
func (h *AppHub) SendChatMessage(message string) {
    h.Clients().All().Send("chatMessageReceived", message)
}
```

These functions must be public so that they can be seen by the signalr server package but can be invoked client-side as lowercase message names.  We'll explain setting up the client side in a moment, but as a preview, here's an example of calling our `AppHub.SendChatMessage(...)` method from the client:

```js
    // javascript snippet invoking that AppHub.Send method from the client
    connection.invoke('sendChatMessage', val);
```

The `signalr.HubInterface` contains a pair of methods you can implement to handle connection and disconnection events.  `signalr.Hub` contains empty implementations of them to satisfy the interface, but you can "override" those defaults by implementing your own functions with your custom hub type as a receiver:

```go
func (c *chat) OnConnected(connectionID string) {
    fmt.Printf("%s connected\n", connectionID)
}

func (c *chat) OnDisconnected(connectionID string) {
   fmt.Printf("%s disconnected\n", connectionID)
}
```

#### Serve with http.ServeMux

```go
import (
    "net/http"
	
    "github.com/philippseith/signalr"
)

func runHTTPServer() {
    address := 'localhost:8080'
    
    // create an instance of your hub
    hub := AppHub{}
	
    // build a signalr.Server using your hub
    // and any server options you may need
    server, _ := signalr.NewServer(context.TODO(),
        signalr.SimpleHubFactory(hub)
        signalr.KeepAliveInterval(2*time.Second),
        signalr.Logger(kitlog.NewLogfmtLogger(os.Stderr), true))
    )
    
    // create a new http.ServerMux to handle your app's http requests
    router := http.NewServeMux()
    
    // ask the signalr server to map it's server
    // api routes to your custom baseurl
    server.MapHTTP(signalr.WithHTTPServeMux(router), "/chat")

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
}
```

### Client side: JavaScript/TypeScript

#### Grab copies of the signalr scripts

Microsoft has published the client-side libraries as a node package with embedded typescript annotations: `@microsoft/signalr`.

You can install `@microsoft/signalr` through any node package manager:

| package manager | command |
| --------------- | ------- |
| [npm](https://www.npmjs.com/) | `npm install @microsoft/signalr@latest` |
| [yarn](https://yarnpkg.com/) | `yarn add @microsoft/signalr@latest` |
| [LibMan](https://docs.microsoft.com/en-us/aspnet/core/client-side/libman/libman-cli)| `libman install @microsoft/signalr@latest -p unpkg -d wwwroot/js/signalr --files dist/browser/signalr.js --files dist/browser/signalr.min.js --files dist/browser/signalr.map.js` |
| none | you can download the version we are using in our `chatsample` from [here](https://raw.githubusercontent.com/philippseith/signalr/master/chatsample/public/js/signalr.js) (the minified version is [here](https://raw.githubusercontent.com/philippseith/signalr/master/chatsample/public/js/signalr.min.js))|

#### Use a HubConnection to connect to your server Hub

How you format your client UI is going to depend on your application use case but here is a simple example.  It illustrates the basic steps of connecting to your server hub:

1. import the `signalr.js` library (or `signalr.min.js`);
1. create a connection object using the `HubConnectionBuilder`;
1. bind events
   - UI event handlers can use `connection.invoke(targetMethod, payload)` to send invoke functions on the server hub;
   - connection event handlers can react to the messages sent from the server hub;
   
1. start your connection

```html
<html>
<body>
    <!-- you may want the content you send to be dynamic -->
    <input type="text" id="message" />
    
    <!-- you may need a trigger to initiate the send -->
    <input type="button" value="Send" id="send" />
    
    <!-- you may want some container to display received messages -->
    <ul id="messages">
    </ul>

    <!-- 1. you need to import the signalr script which provides
            the HubConnectionBuilder and handles the connection
            plumbing.
    -->
    <script src="js/signalr.js"></script>
    <script>
    (async function () {
        // 2. use the signalr.HubConnectionBuilder to build a hub connection
        //    and point it at the baseurl which you configured in your mux
        const connection = new signalR.HubConnectionBuilder()
                .withUrl('/chat')
                .build();

        // 3. bind events:
        //    - UI events can invoke (i.e. dispatch to) functions on the server hub
        document.getElementById('send').addEventListener('click', sendClicked);
        //    - connection events can handle messages received from the server hub
        connection.on('chatMessageReceived', onChatMessageReceived);

        // 4. call start to initiate the connection and start streaming events
        //    between your server hub and your client connection
        connection.start();
        
        // that's it! your server and client should be able to communicate
        // through the signalr.Hub <--> connection pipeline managed by the
        // signalr package and client-side library.
        
        // --------------------------------------------------------------------
       
        // example UI event handler
        function sendClicked() {
            // prepare your target payload
            const msg = document.getElementById('message').value;
            if (msg) {
                // call invoke on your connection object to dispatch
                // messages to the server hub with two arguments:
                // -  target: name of the hub func to invoke
                // - payload: the message body
                // 
                const target = 'sendChatMessage';
                connection.invoke(target, msg);
            }
        }

        // example server event handler
        function onChatMessageReceived(payload) {
            // the payload is whatever was passed to the inner
            // clients' `Send(...)` method in your server-side
            // hub function.
           
            const li = document.createElement('li');
            li.innerText = payload;
            document.getElementById('messages').appendChild(li);
        }
    })();
    </script>
</body>
</html>
```

### Client side: go

To handle callbacks from the server, create a receiver class which gets the server callbacks mapped
to its methods:
```go
type receiver struct {
	signalr.Hub
}

func (c *receiver) Receive(msg string) {
	fmt.Println(msg)
}
```
`Receive` gets called when the server does something like this:
```go
hub.Clients().Caller().Send("receive", message)
```

The client itself might be used like that:
```go
// Create a Connection (with timeout for the negotiation process)
creationCtx, _ := context.WithTimeout(ctx, 2 * time.Second)
conn, err := signalr.NewHTTPConnection(creationCtx, address)
if err != nil {
    return err
}
// Create the client and set a receiver for callbacks from the server
client, err := signalr.NewClient(ctx,
	signalr.WithConnection(conn),
	signalr.WithReceiver(receiver))
if err != nil {
    return err
}
// Start the client loop
c.Start()
// Do some client work
ch := <-c.Invoke("update", data)
// ch gets the result of the update operation
```

## Debugging

Server, Client and the protocol implementations are able to log most of their operations. The logging option is disabled
by default in all tests. To configure logging, edit the `testLogConf.json` file:
```json
{
  "Enabled": false,
  "Debug": false
}
```
- If `Enabled` is set to `true`, the logging will be enabled. The tests will log to `os.Stderr`.
- If `Debug` ist set to `true`, the logging will be more detailed.
