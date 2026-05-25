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
	if os.Getenv("ECHOTRAILS_AUTH_EMAIL_DEV") != "1" {
		log.Fatal("no email provider configured; set ECHOTRAILS_AUTH_EMAIL_DEV=1 only for local development")
	}

	server := api.NewServer(api.Config{Sender: api.DevEmailSender{Log: log.Printf}})
	log.Printf("starting echotrails api on %s", addr)
	if err := http.ListenAndServe(addr, server.Router()); err != nil {
		log.Fatal(err)
	}
}
