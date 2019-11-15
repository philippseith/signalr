const ws = require('ws');
const crypto = require('crypto');
const signalr = require('@microsoft/signalr');
const url = require('url');


// TODO: Make configurable
const handshakeTimeoutMs = 5000;
const pingIntervalMs = 5000;
const protocols = {
    json: new signalr.JsonHubProtocol()
};

const wss = new ws.Server({ noServer: true });

// TODO: SignalR should expose HandshakeProtocol
const TextMessageFormat = {
    RecordSeparator: String.fromCharCode(0x1e),

    write: function (output) {
        return `${output}${TextMessageFormat.RecordSeparator}`;
    },

    parse: function (input) {
        if (input[input.length - 1] !== TextMessageFormat.RecordSeparator) {
            throw new Error("Message is incomplete.");
        }

        const messages = input.split(TextMessageFormat.RecordSeparator);
        messages.pop();
        return messages;
    }
}

class Connection {
    constructor(id, ws) {
        this.id = id;
        this.ws = ws;
        this.protocol = null;
        this._timer = null;
        this._serializedPingMessage = null;
    }

    setProtocol(protocol) {
        this.protocol = protocol;
        this._serializedPingMessage = protocol.writeMessage({ type: 6 });
    }

    parseMessages(data) {
        return this.protocol.parseMessages(data);
    }

    sendInvocation(target, args) {
        var obj = { type: 1, target: target, arguments: args };
        this.ws.send(this.protocol.writeMessage(obj));
    }

    doHandshakeResponse(error) {
        var obj = {};
        if (error) {
            obj['error'] = error;
        }
        this.ws.send(TextMessageFormat.write(JSON.stringify(obj)));
    }

    ping() {
        this.ws.send(this._serializedPingMessage);
    }

    completion(id, result, error) {
        var obj = { type: 3, invocationId: id };
        if (result) {
            obj['result'] = result;
        }

        if (error) {
            obj['error'] = error;
        }

        this.ws.send(this.protocol.writeMessage(obj));
    }

    start() {
        // This can't be efficient can it?
        this._timer = setInterval(() => {
            this.ping();
        },
            pingIntervalMs);
    }

    stop() {
        clearInterval(this._timer);
    }

    on(name, handler) {
        // TODO: Handle partial messages and buffering here, don't trigger the handler until we have a full message
        this.ws.on(name, handler);
    }

    close() {
        this.ws.close();
    }
}

class HubConnectionHandler {
    constructor(dispatcher, lifetimeManager) {
        this._lifetimeManager = lifetimeManager;
        this._dispatcher = dispatcher;
    }

    onSocketConnect(connection) {
        var handshake = false;

        var handshakeTimeout = setTimeout(() => {
            if (!handshake) {
                connection.close();
            }
        }, handshakeTimeoutMs);

        connection.on('message', (message) => {
            if (!handshake) {
                // TODO: This needs to handle partial data and multiple messages
                var messages = TextMessageFormat.parse(message);

                var handshakeMessage = JSON.parse(messages[0]);
                var protocol = protocols[handshakeMessage.protocol];

                // Cancel the timeout
                clearInterval(handshakeTimeout);

                if (!protocol) {
                    // Fail for anything but JSON right now
                    connection.doHandshakeResponse(`Requested protocol '${handshakeMessage.protocol}' is not available.`);
                }
                else {
                    connection.setProtocol(protocol);

                    // All good!
                    connection.doHandshakeResponse();
                    handshake = true;

                    connection.start();

                    // Now we're connected
                    this._lifetimeManager.onConnect(connection);
                    this._dispatcher._onConnect(connection.id);
                }
            }
            else {
                var messages = connection.parseMessages(message);

                for (const message of messages) {
                    switch (message.type) {
                        case signalr.MessageType.Close:
                            console.debug("Close message received from server.");

                            // We don't want to wait on the stop itself.
                            // this.stopPromise = this.stopInternal(message.error ? new Error("Server returned an error on close: " + message.error) : undefined);
                            // connection.close();
                            break;
                        default:
                            this._dispatcher._dispatch(connection, message);
                            break;
                    }
                }
            }

            // console.debug(message);
        });

        connection.on('close', () => {
            connection.stop();

            this._lifetimeManager.onDisconnect(connection);
            this._dispatcher._onDisconnect(connection.id);
        });
    }
}

class HubLifetimeManager {
    constructor() {
        this._clients = new Map();
    }

    onConnect(connection) {
        this._clients[connection.id] = connection;
    }

    invokeAll(target, args) {
        for (const key in this._clients) {
            var connection = this._clients[key];
            connection.sendInvocation(target, args);
        }
    }

    invokeClient(id, target, args) {
        var connection = this._clients[id];
        if (connection) {
            connection.sendInvocation(target, args);
        }
    }

    onDisconnect(connection) {
        delete this._clients[connection.id];
    }
}

class AllClientProxy {
    constructor(lifetimeManager) {
        this._lifetimeManager = lifetimeManager;
    }

    send(name, ...args) {
        this._lifetimeManager.invokeAll(name, args);
    }
}

class SingleClientProxy {
    constructor(id, lifetimeManager) {
        this.id = id;
        this._lifetimeManager = lifetimeManager;
    }

    send(name, ...args) {
        this._lifetimeManager.invokeClient(this.id, name, args);
    }
}

class HubClients {
    constructor(lifetimeManager) {
        this.all = new AllClientProxy(lifetimeManager);
    }
    client(id) {
        return new SingleClientProxy(id);
    }
}

class Hub {
    _map = new Map();
    _lifetimeManager = new HubLifetimeManager();
    _connectionHandler = null;

    constructor() {
        this._connectCallback = null;
        this._disconnectCallback = null;
        this._connectionHandler = new HubConnectionHandler(this, this._lifetimeManager);
        this.clients = new HubClients(this._lifetimeManager);
    }

    on(method, handler) {
        if (method === 'connect') {
            this._connectCallback = handler;
        }
        else if (method === 'disconnect') {
            this._disconnectCallback = handler;
        }
        this._map[method] = handler;
    }

    _onConnect(id) {
        if (this._connectCallback) {
            this._connectCallback.apply(this, [id]);
        }
    }

    _onDisconnect(id) {
        if (this._disconnectCallback) {
            this._disconnectCallback.apply(this, [id]);
        }
    }

    // Dispatcher should be decoupled from the hub but there are layering issues
    _dispatch(connection, message) {
        switch (message.type) {
            case signalr.MessageType.Invocation:
                // TODO: Handle async methods?
                try {
                    var method = this._map[message.target.toLowerCase()];
                    var result = method.apply(this, message.arguments);
                    connection.completion(message.invocationId, result);
                }
                catch (e) {
                    connection.completion(message.invocationId, null, 'There was an error invoking the hub');
                }
                break;
            case signalr.MessageType.StreamItem:
                break;
            case signalr.MessageType.Ping:
                // TODO: Detect client timeout
                break;
            default:
                console.error(`Invalid message type: ${message.type}.`);
                break;
        }
    }
}

function normalize(path) {
    if (path[path.length - 1] != '/') {
        path += '/';
    }
    return path;
}

function matches(req, method, parsedUrl, path) {
    return req.method == method && path === normalize(parsedUrl.pathname.substr(0, path.length));
}

// TODO: de-couple from express
function mapHub(hub, server, path) {
    var listeners = server.listeners('request').slice(0);
    server.removeAllListeners('request');

    path = normalize(path);
    var negotiatePath = `${path}negotiate/`;

    server.on('request', (req, res) => {
        var parsedUrl = url.parse(req.url, true);

        if (matches(req, 'POST', parsedUrl, negotiatePath)) {
            var bytes = crypto.randomBytes(16);
            var connectionId = bytes.toString('base64');

            // Only websockets
            res.end(JSON.stringify({
                connectionId: connectionId,
                availableTransports: [
                    {
                        transport: "WebSockets",
                        transferFormats: ["Text", "Binary"]
                    }
                ]
            }));
        }
        else if (matches(req, 'GET', parsedUrl, path)) {
            if (req.headers["connection"] != "Upgrade") {
                res.sendStatus(400);
                return;
            }

            var id = parsedUrl.query.id;

            // Check for upgrade
            wss.handleUpgrade(req, req.socket, Buffer.alloc(0), (ws) => {
                hub._connectionHandler.onSocketConnect(new Connection(id, ws));
            });
        }
        else {
            for (var i = 0, l = listeners.length; i < l; i++) {
                listeners[i].call(server, req, res);
            }
        }
    });
}

var hubs = new Map();

module.exports = function name(app) {
    return {
        mapHub: (path) => {
            var hub = hubs[path];
            if (!hub) {
                hub = new Hub(app, path);
                mapHub(hub, app, path);
            }
            return hub;
        }
    };
};