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
        this._ws = ws;
        this._handshake = false;
        this._protocol = null;
        this._timer = null;
        this._serializedPingMessage = null;
        this._handshakeCompleteHandler = null;
        this._messageHandler = null;
        this._closeHandler = null;

        this._handshakeTimeout = setTimeout(() => {
            if (!this._handshake) {
                this._ws.close();
            }
        }, handshakeTimeoutMs);

        this._ws.on('message', (message) => this._onWsMessage(message));
        this._ws.on('close', () => {
            this._stop();
            if (this._closeHandler) {
                this._closeHandler.apply(this);
            }
        });
    }

    sendInvocation(target, args) {
        this._ws.send(this.getInvocation(target, args));
    }

    getInvocation(target, args) {
        var obj = { type: 1, target: target, arguments: args };
        return this._protocol.writeMessage(obj);
    }

    sendRawMessage(raw) {
        this._ws.send(raw);
    }

    completion(id, result, error) {
        var obj = { type: 3, invocationId: id };
        if (result) {
            obj['result'] = result;
        }

        if (error) {
            obj['error'] = error;
        }

        this._ws.send(this._protocol.writeMessage(obj));
    }

    onHandshakeComplete(handler) {
        this._handshakeCompleteHandler = handler;
    }

    onMessage(handler) {
        this._messageHandler = handler;
    }

    onClose(handler) {
        this._closeHandler = handler;
    }

    close() {
        this._ws.close();
    }

    _setProtocol(protocol) {
        this._protocol = protocol;
        this._serializedPingMessage = protocol.writeMessage({ type: 6 });
    }

    _parseMessages(data) {
        return this._protocol.parseMessages(data);
    }

    _doHandshakeResponse(error) {
        var obj = {};
        if (error) {
            obj['error'] = error;
        }
        this._ws.send(TextMessageFormat.write(JSON.stringify(obj)));
    }

    _ping() {
        this._ws.send(this._serializedPingMessage);
    }

    _onWsMessage(message) {
        if (!this._handshake) {
            // TODO: This needs to handle partial data and multiple messages
            var messages = TextMessageFormat.parse(message);

            var handshakeMessage = JSON.parse(messages[0]);
            var protocol = protocols[handshakeMessage.protocol];

            // Cancel the timeout
            clearInterval(this._handshakeTimeout);

            if (!protocol) {
                // Fail for anything but JSON right now
                this._doHandshakeResponse(`Requested protocol '${handshakeMessage.protocol}' is not available.`);
            }
            else {
                this._setProtocol(protocol);

                // All good!
                this._doHandshakeResponse();
                this._handshake = true;

                this._start();

                if (this._handshakeCompleteHandler) {
                    this._handshakeCompleteHandler.apply(this);
                }
            }
        }
        else {
            var messages = this._parseMessages(message);

            for (const message of messages) {
                if (this._messageHandler) {
                    this._messageHandler(message);
                }
            }
        }
    }

    _start() {
        // This can't be efficient can it?
        this._timer = setInterval(() => {
            this._ping();
        }, pingIntervalMs);
    }

    _stop() {
        clearInterval(this._timer);
    }
}

class HubConnectionHandler {
    constructor(dispatcher, lifetimeManager) {
        this._lifetimeManager = lifetimeManager;
        this._dispatcher = dispatcher;
    }

    onSocketConnect(connection) {
        connection.onHandshakeComplete(() => {
            // Now we're connected
            this._lifetimeManager.onConnect(connection);
            this._dispatcher._onConnect(connection.id);
        });

        connection.onMessage(message => {
            this._dispatcher._onMessage(connection, message);
        });

        connection.onClose(() => {
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
    _methods = new Map();

    constructor() {
        this._connectCallback = null;
        this._disconnectCallback = null;
        this.clients = null;
    }

    on(method, handler) {
        if (method === 'connect') {
            this._connectCallback = handler;
        }
        else if (method === 'disconnect') {
            this._disconnectCallback = handler;
        }
        else {
            this._methods[method] = handler;
        }
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
    _onMessage(connection, message) {
        switch (message.type) {
            case signalr.MessageType.Invocation:
                // TODO: Handle async methods?
                try {
                    var method = this._methods[message.target.toLowerCase()];
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

function mapHub(hub, server, path, lifetimeManager) {
    var connectionHandler = new HubConnectionHandler(hub, lifetimeManager);
    hub.clients = new HubClients(lifetimeManager);

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
                connectionHandler.onSocketConnect(new Connection(id, ws));
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
// TODO: This should be pluggable
var lifetimeManager = new HubLifetimeManager();

module.exports = function name(httpServer) {
    return {
        mapHub: (path) => {
            var hub = hubs[path];
            if (!hub) {
                hub = new Hub();
                mapHub(hub, httpServer, path, lifetimeManager);
            }
            return hub;
        }
    };
};