package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WS upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Starts the metric http server with websocket.
func (ds *ReportingData) startServer() {

	// Serving index.html
	fs := http.FileServer(http.Dir("./ui"))

	http.Handle("/", fs)

	// Exposing websocket
	http.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)

		if err != nil {
			fmt.Println(err)
			return
		}
		for {
			defer conn.Close()
			// Write data collected every 5 seconds through websocket
			err = conn.WriteJSON(ds)
			if err != nil {
				fmt.Println(err)
				return
			}
			time.Sleep(5 * time.Second)

		}
	})

	http.ListenAndServe(":3000", nil)
	// http.ListenAndServeTLS(":3000", "cert.pem", "key.pem", nil)
}
