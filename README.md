## SignalR

[![Actions Status](https://github.com/philippseith/signalr/workflows/Build%20and%20Test/badge.svg)](https://github.com/philippseith/signalr/actions)
[![codecov](https://codecov.io/gh/philippseith/signalr/branch/master/graph/badge.svg)](https://codecov.io/gh/philippseith/signalr)
[![GoDoc](https://godoc.org/github.com/philippseith/signalr?status.svg)](https://godoc.org/github.com/philippseith/signalr)
[![Go Report Card](https://goreportcard.com/badge/github.com/philippseith/signalr)](https://goreportcard.com/report/github.com/philippseith/signalr)
[![HitCount](http://hits.dwyl.com/philippseith/https://githubcom/philippseith/signalr.svg)](http://hits.dwyl.com/philippseith/https://githubcom/philippseith/signalr)

SignalR is an open-source library that simplifies adding real-time web functionality to apps. 
Real-time web functionality enables server-side code to push content to clients instantly.

Historically it was tied to ASP.NET Core but the 
[protocol](https://github.com/aspnet/AspNetCore/tree/master/src/SignalR/docs/specs) is open and implementable in any language.

This repository contains an implementation of a SignalR server and a SignalR client in go. 
The implementation is based on the work of David Fowler at https://github.com/davidfowl/signalr-ports.
Client and server support transport over WebSockets, Server Sent Events and raw TCP.
Protocol encoding in JSON is fully supported, and there is MessagePack support for basic types.
