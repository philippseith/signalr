const express = require('express');
const app = express();
const http = require('http').createServer(app);
const signalR = require('./signalr')(http);
const port = 3000;

const chat = signalR.mapHub('/chat');

chat.on('connect', (id) => {
    chat.groups.addToGroup(id, "MyGroup");
    console.log(`${id} connected`);
});

chat.on('disconnect', (id) => {
    console.log(`${id} disconnected`);
});

chat.on('send', (message) => {
    chat.clients.group("MyGroup").send('send', message);
    return "Hello World";
});

app.use(express.static('public'));

http.listen(port, () => {
    console.log(`listening on *:${port}`);
});

