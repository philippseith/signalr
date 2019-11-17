const express = require('express');
const app = express();
const http = require('http').createServer(app);
const signalR = require('./signalr')(http);
const port = 3000;

const chat = signalR.mapHub('/chat');
const clock = signalR.mapHub('/clock');

setInterval(() => {
    clock.clients.all.send('tick', Date.now());
}, 1000);

chat.on('connect', (id) => {
    console.log(`${id} connected`);
});

chat.on('disconnect', (id) => {
    console.log(`${id} disconnected`);
});

chat.on('send', (message) => {
    chat.clients.all.send('send', message);
});

app.use(express.static('public'));

http.listen(port, () => {
    console.log(`listening on *:${port}`);
});

