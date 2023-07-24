package main

import (
	"fmt"
	proxy_server "g-proxy/gproxy"
	"log"
	"net/http"
	_ "net/http/pprof"
)

func main() {
	go http.ListenAndServe(":8888", nil)
	server := proxy_server.NewProxyServer()
	fmt.Printf("Proxy Server Running \n")
	if err := http.ListenAndServe(":18085", server); err != nil {
		log.Fatalf("could not listen on port 18085 %v", err)
	}
}
