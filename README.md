# Wemdigo  

**We**bsocket **M**i**d**dlelayer in **Go**.  

[Docs](https://godoc.org/github.com/shawnohare/wemdigo)


This package makes the creation and managing of multiple Gorilla
WebSocket connections easier, while also allowing for intermediate
message interception. One use case for a `wemdigo.Middle` instance is
to route messages from a front-end to multiple back-end microservices.

## Client-server middle layer example 

A minimal example where the Middle layer blindly passes messages
between a client and a backend service.

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
	// so the package's default one is used. The Peers field is set
	// so that the the Middle layer will automatically close one websocket
	// connection when the other has closed. For this example, the Peer
	// field can be omitted for the same effect.
	conf := wemdigo.Config{
		Conns: map[string]*websocket.Conn{
			"client": client,
			"server": server,
		},
		Peers: map[string][]string{
			"client": {"server"},
			"server": {"client"},
		},
		PingPeriod: 20 * time.Second,
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








