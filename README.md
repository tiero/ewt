# Elements Wallet Tracker
Lightweight personal wallet indexer for Elements/Liquid blockchain


## `TL;DR` 

> Give me an xpub, will notify you when you got a new utxo


## Overview

- Lightweight: just connect to your own elements node
- Esplora REST API compatible
- Protobuf/gRPC interface for HTTP/2 binary streaming 


## Usage

TBD

## Development

Get packages

```sh
$ go get -v ./...
```

Run a basic ZMQ event consumer with hardcoded script

```sh
$ make run
Run...
Hello from ewt daemon
INFO[0000] Started listening for elementsd transaction notifications via ZMQ on: 127.0.0.1:28333 
INFO[0000] Started listening for elementsd block notifications via ZMQ on: 127.0.0.1:28332 
```

On another tab send funs to the address for that script

```
$ nigiri faucet --liquid el1qq06zzu3cs53n3ufhhykgtqjval9w0a2gnufjux9fxj4d43hzdl9fqcv7e7enwyxtn0qajw7lfnwcxv80grx3s8vlk4wu0tvap
```


On the other tab you should see the following

```sh
...
INFO[0066] Received new tx with id e2af39a52b776d406d2df7eef04df101b4dbc89c6317e52202c7c9e259197dc8 at time 2021-02-25 01:56:15.285610323 +0100 CET m=+66.890936802
```