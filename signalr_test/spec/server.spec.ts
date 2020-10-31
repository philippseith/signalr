import * as signalR from "@aspnet/signalr";

// IMPORTANT: When a proxy (e.g. px) is in use, the server will get the request,
// but the client will not get the response
// So disable the proxy for this process.
process.env.http_proxy = "";

const builder: signalR.HubConnectionBuilder = new signalR.HubConnectionBuilder().configureLogging(signalR.LogLevel.Debug);

describe("signalr server at /hub", () => {
    it("should connect on a clients request for connection", async () => {
        const connection: signalR.HubConnection = builder
            .withUrl("http://127.0.0.1:5000/hub")
            .build();
        await connection.start();
        await connection.stop();
    });
});
