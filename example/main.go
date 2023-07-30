package main

import (
	"fmt"
	gproxy "g-proxy"
	"log"
	"net/http"
	_ "net/http/pprof"
)

func main() {
	go http.ListenAndServe(":8888", nil)
	server := gproxy.NewProxyServer()
	fmt.Printf("Proxy Server Running \n")
	if err := http.ListenAndServe(":18085", server); err != nil {
		log.Fatalf("could not listen on port 18085 %v", err)
	}
}
