package main

import (
	proxy_server "g-proxy/proxy_server"
	"log"
	"net/http"
)

func main() {
	server := proxy_server.NewProxyServer()

	if err := http.ListenAndServe(":18085", server); err != nil {
		log.Fatalf("could not listen on port 18085 %v", err)
	}
}
