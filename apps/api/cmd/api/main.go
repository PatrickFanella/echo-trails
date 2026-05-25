package main

import (
	"log"
	"net/http"
	"os"

	"echotrails/internal/api"
)

func main() {
	addr := os.Getenv("ECHOTRAILS_API_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	server := api.NewServer(api.Config{})
	log.Printf("starting echotrails api on %s", addr)
	if err := http.ListenAndServe(addr, server.Router()); err != nil {
		log.Fatal(err)
	}
}
