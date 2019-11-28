## SignalR Node Server

### Installation
Make sure you run Node version >= 12. Check the release schedule for LTS versions: https://github.com/nodejs/Release
```npm i && npm start```

### Docker
```
docker build -t signalr-node .
docker run -p 3000:3000 signalr-node
```