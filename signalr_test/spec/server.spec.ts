import * as signalR from '@microsoft/signalr';
import { Subject, from } from 'rxjs'
import { eachValueFrom } from 'rxjs-for-await';

// IMPORTANT: When a proxy (e.g. px) is in use, the server will get the request,
// but the client will not get the response
// So disable the proxy for this process.
process.env.http_proxy = "";

const builder: signalR.HubConnectionBuilder =
    new signalR.HubConnectionBuilder().configureLogging(signalR.LogLevel.Debug);

describe("smoke test", () => {
    it("should connect on a clients request for connection and answer a simple request",
        async () => {
            const connection: signalR.HubConnection = builder
                .withUrl("http://127.0.0.1:5000/hub")
                .build();
            await connection.start();
            const pong = await connection.invoke("ping");
            expect(pong).toEqual("Pong");
            await connection.stop();
        });
});

class AlcoholicContent {
    drink: string
    strength: number
}

describe("e2e test with aspnet/signalr client", () =>{
    let connection: signalR.HubConnection;
    beforeEach(async() => {
        connection = builder
            .withUrl("http://127.0.0.1:5000/hub")
            .build();
        await connection.start();
    })
    afterEach(async() => {
        await connection.stop();
    })
    it("should answer a simple request", async () => {
        const pong = await connection.invoke("ping");
        expect(pong).toEqual("Pong");
    })
    it("should answer a simple request with multiple results", async () => {
        const triple = await connection.invoke("triumphantTriple", "1.FC Bayern München");
        expect(triple).toEqual(["German Championship", "DFB Cup", "Champions League"]);
    })
    it("should answer a request with an resulting array of structs", async () => {
        const contents: AlcoholicContent[] = await connection.invoke("alcoholicContents");
        expect(contents.length).toEqual(3);
        expect(contents[0].drink).toEqual('Brunello');
        expect(contents[2].strength).toEqual(56.2);
    })
    it("should answer a request with an resulting map", async () => {
        const contents = await connection.invoke("alcoholicContentMap");
        expect(contents["Beer"]).toEqual(4.9);
        expect(contents["Lagavulin Cask Strength"]).toEqual(56.2);
    })

    it("should receive a stream", async () => {
        const fiveDates: Subject<string> = new Subject<string>();
        connection.stream<string>("FiveDates").subscribe(fiveDates);
        let i = 0;
        let lastValue = '';
        for await (const value of eachValueFrom(fiveDates)) {
            expect(value).toBeDefined();
            expect(value).not.toEqual(lastValue);
            lastValue = value;
            i++;
        }
        expect(i).toEqual(5)
    })
    it("should upload a stream", async() =>{
        let received: number[];
        const receive = new Promise<number[]>(resolve => {
            connection.on("onUploadComplete", (r: number[]) => {
                received = r;
                resolve();
            });
        });
        await connection.send("uploadStream", from([2, 0, 7]));
        await receive;
        expect(received).toEqual([2, 0, 7])
    })
});