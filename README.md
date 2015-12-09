# Wemdigo  [![GoDoc](https://godoc.org/github.com/shawnohare/wemdigo?status.svg)](http://godoc.org/github.com/shawnohare/wemdigo)
[![Circle CI](https://circleci.com/gh/shawnohare/wemdigo.svg?style=svg)](https://circleci.com/gh/shawnohare/wemdigo)
**We**bsocket **M**i**d**dlelayer in **Go**.  

This package makes the creation and managing of multiple Gorilla
WebSocket connections easier, while also allowing for intermediate
message interception. One use case for a `wemdigo.Middle` instance is
to route messages from a front-end to multiple back-end microservices.

## Client-server middle layer example 

A small  example where the Middle layer blindly passes messages
between a client and a backend service.  Each time a client HTTP
request is upgraded to a websocket connection, it's dual websocket with
a backend service is created and these two websockets are linked. **Note**
that in this example, each time a HTTP request is upgraded, a new 
Middle layer between the client and server is created.

```go
package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/shawnohare/wemdigo"
)

func ServiceHandler(w http.ResponseWriter, r *http.Request) {

	// Establish socket connection to client.
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Upgrade to a websocket.
	client, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Establish socket connection to the backend service.
	addr := "localhost:8080"
	path := "/service"
	u := url.URL{Scheme: "ws", Host: addr, Path: path}
	log.Printf("connecting to %s", u.String())
	server, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}

	// Specify the wemdigo Middle-layer configuration.  This
	// Configuration does not specify a Message handler function,
	// so the package's default one is used.  The client connection
  // will automatically close when the server connection closes, due
  // to the Deps setting.
	conf := wemdigo.Config{
		ConnConfigs: []wemdigo.ConnConfig{
			{
				Conn:    client,
				Key:     "client",
				Targets: []string{"server"},
				Deps:    []string{"server"},
			},
			{
				Conn:    server,
				Key:     "server",
				Targets: []string{"client"},
			},
		},
		Handler: rm.messageHandler,
	}
	m := wemdigo.New(conf)
	m.Run()
}

func main() {
	log.Println("App using a wemdigo Middle layer.")

	router := mux.NewRouter()
	router.HandleFunc("/service", ServiceHandler)

	n := negroni.Classic()
	n.UseHandler(router)

	// Start Server.
	if os.Getenv("PORT") != "" {
		n.Run(strings.Join([]string{":", os.Getenv("PORT")}, ""))
	} else {
		n.Run(":8081")
	}

}

```








