const signalr = require('@microsoft/signalr');
const signalrHttp = require("./signalr-http");

// TODO: Make configurable
const handshakeTimeoutMs = 5000;
const pingIntervalMs = 5000;
const protocols = {
    json: new signalr.JsonHubProtocol()
};

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

class HubConnection {
    constructor(connection) {
        this._connection = connection;
        this._handshake = false;
        this._protocol = null;
        this._timer = null;
        this._serializedPingMessage = null;
        this._handshakeCompleteHandler = null;
        this._messageHandler = null;
        this._closeHandler = null;

        this._handshakeTimeout = setTimeout(() => {
            if (!this._handshake) {
                this.connection.close();
            }
        }, handshakeTimeoutMs);

        this._connection.onmessage((message) => this._onMessage(message));
        this._connection.onclose(() => {
            this._stop();
            if (this._closeHandler) {
                this._closeHandler.apply(this);
            }
        });
    }

    get id() {
        return this._connection.id;
    }

    sendInvocation(target, args) {
        this._connection.send(this.getInvocation(target, args));
    }

    getInvocation(target, args) {
        var obj = { type: 1, target: target, arguments: args };
        return this._protocol.writeMessage(obj);
    }

    sendRawMessage(raw) {
        this._connection.send(raw);
    }

    completion(id, result, error) {
        var obj = { type: 3, invocationId: id };
        if (result) {
            obj['result'] = result;
        }

        if (error) {
            obj['error'] = error;
        }

        this._connection.send(this._protocol.writeMessage(obj));
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
        this._connection.close();
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
        this._connection.send(TextMessageFormat.write(JSON.stringify(obj)));
    }

    _ping() {
        this._connection.send(this._serializedPingMessage);
    }

    _onMessage(message) {
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

    onConnect(connection) {
        // What's the lifetime of this thing...
        var hubConnection = new HubConnection(connection);

        hubConnection.onHandshakeComplete(() => {
            // Now we're connected
            this._lifetimeManager.onConnect(hubConnection);
            this._dispatcher._onConnect(hubConnection.id);
        });

        hubConnection.onMessage(message => {
            this._dispatcher._onMessage(hubConnection, message);
        });

        hubConnection.onClose(() => {
            this._lifetimeManager.onDisconnect(hubConnection);
            this._dispatcher._onDisconnect(hubConnection.id);
        });
    }
}

class HubLifetimeManager {
    constructor() {
        this._clients = new Map();
        this._groups = new Map();
    }

    onConnect(connection) {
        this._clients[connection.id] = {
            connection: connection,
            groups: []
        };
    }

    invokeAll(target, args) {
        for (const key in this._clients) {
            var client = this._clients[key];
            client.connection.sendInvocation(target, args);
        }
    }

    invokeClient(id, target, args) {
        var client = this._clients[id];
        if (client) {
            client.connection.sendInvocation(target, args);
        }
    }

    invokeGroup(groupName, target, args) {
        var group = this._groups[groupName];

        if (!group) {
            return;
        }

        for (const key in group) {
            var client = group[key];
            client.connection.sendInvocation(target, args);
        }
    }

    addToGroup(id, groupName) {
        var client = this._clients[id];
        if (!client) {
            return;
        }
        var group = this._groups[groupName];
        if (!group) {
            group = new Map();
            this._groups[groupName] = group;
        }

        // Store the group name
        client.groups.push(groupName);

        group[id] = client;
    }

    removeFromGroup(id, groupName) {
        var group = this._groups[groupName];
        if (!group) {
            return;
        }

        delete group[id];

        if (group.size === 0) {
            delete this._groups[groupName];
        }
    }

    onDisconnect(connection) {
        var client = this._clients[connection.id];
        delete this._clients[connection.id];
        
        // Clean up groups
        for (const group of client.groups) {
            // REVIEW: Performance..
            this.removeFromGroup(connection.id, group);
        }
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

class GroupClientProxy {
    constructor(groupName, lifetimeManager) {
        this._groupName = groupName;
        this._lifetimeManager = lifetimeManager;
    }

    send(name, ...args) {
        this._lifetimeManager.invokeGroup(this._groupName, name, args);
    }
}

class HubClients {
    constructor(lifetimeManager) {
        this._lifetimeManager = lifetimeManager;
        this.all = new AllClientProxy(lifetimeManager);
    }
    client(id) {
        return new SingleClientProxy(id, this._lifetimeManager);
    }
    group(groupName) {
        return new GroupClientProxy(groupName, this._lifetimeManager);
    }
}

class HubGroupManager {
    constructor(lifetimeManager) {
        this._lifetimeManager = lifetimeManager;
    }

    addToGroup(id, group) {
        this._lifetimeManager.addToGroup(id, group);
    }

    removeFromGroup(id, group) {
        this._lifetimeManager.removeFromGroup(id, group);
    }
}

class HubContext {
    constructor(lifetimeManager) {
        this.clients = new HubClients(lifetimeManager);
        this.groups = new HubGroupManager(lifetimeManager);
    }
}

class Hub {
    _methods = new Map();

    constructor() {
        this._connectCallback = null;
        this._disconnectCallback = null;
        this.clients = null;
        this.groups = null;
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
    async _onMessage(connection, message) {
        switch (message.type) {
            case signalr.MessageType.Invocation:
                try {
                    var method = this._methods[message.target.toLowerCase()];
                    var result = await method.apply(this, message.arguments);
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

var hubs = new Map();
var defaultLifetimeManager = new HubLifetimeManager();

module.exports = function name(httpServer) {
    return {
        // Any transport
        hub: (options) => {
            options = options || {};
            hub = new Hub();
            // Resolve the lifetime manager
            var lifetimeManager = options.lifetimeManager || defaultLifetimeManager;
            var transport = options.transport;

            var connectionHandler = new HubConnectionHandler(hub, lifetimeManager);
            var hubContext = new HubContext(lifetimeManager);
            hub.clients = hubContext.clients;
            hub.groups = hubContext.groups;

            transport.start(connectionHandler);

            return hub;
        },
        // Http
        mapHub: (path, options) => {
            options = options || {};
            var hub = hubs[path];
            if (!hub) {
                hub = new Hub();
                // Resolve the lifetime manager
                var lifetimeManager = options.lifetimeManager || defaultLifetimeManager;
                var transport = new signalrHttp.HttpTransport(path, httpServer);

                var connectionHandler = new HubConnectionHandler(hub, lifetimeManager);
                var hubContext = new HubContext(lifetimeManager);
                hub.clients = hubContext.clients;
                hub.groups = hubContext.groups;

                transport.start(connectionHandler);
            }
            return hub;
        }
    };
};