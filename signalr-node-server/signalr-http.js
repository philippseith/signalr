const ws = require('ws');
const crypto = require('crypto');
const url = require('url');

const wss = new ws.Server({ noServer: true });

// This should be an interface
class WebSocketConnection {
    constructor(id, ws) {
        this.id = id;
        this._ws = ws;
    }

    onmessage(handler) {
        this._ws.on('message', handler);
    }

    onclose(handler) {
        this._ws.on('close', handler);
    }

    send(data) {
        this._ws.send(data);
    }

    stop() {
        this._ws.close();
    }
}

class HttpTransport {
    constructor(basePath, httpServer) {
        this._basePath = basePath;
        this._httpServer = httpServer;
    }

    _normalize(path) {
        if (path[path.length - 1] != '/') {
            path += '/';
        }
        return path;
    }

    _matches(req, method, parsedUrl, path) {
        return req.method == method && path === this._normalize(parsedUrl.pathname.substr(0, path.length));
    }

    start(connectionHandler) {
        var listeners = this._httpServer.listeners('request').slice(0);
        this._httpServer.removeAllListeners('request');

        var path = this._normalize(this._basePath);
        var negotiatePath = `${path}negotiate/`;

        this._httpServer.on('request', (req, res) => {
            var parsedUrl = url.parse(req.url, true);

            if (this._matches(req, 'POST', parsedUrl, negotiatePath)) {
                var bytes = crypto.randomBytes(16);
                var connectionId = bytes.toString('base64');

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
            
            else if (this._matches(req, 'GET', parsedUrl, path)) {
                if (req.headers["connection"] != "Upgrade") {
                    res.sendStatus(400);
                    return;
                }

                var id = parsedUrl.query.id;

                // Check for upgrade
                wss.handleUpgrade(req, req.socket, Buffer.alloc(0), (ws) => {
                    connectionHandler.onConnect(new WebSocketConnection(id, ws));
                });
            }
            else {
                for (var i = 0, l = listeners.length; i < l; i++) {
                    listeners[i].call(this._httpServer, req, res);
                }
            }
        });
    }
}

module.exports.HttpTransport = HttpTransport;