package main

import (
	"log"
	"net/http"
	"os"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/api"
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
	places := api.NewHandler(api.NewStore(api.DefaultPlaces()))
	mux := http.NewServeMux()
	mux.Handle("/places/search", places)
	mux.Handle("/places/requests", places)
	mux.Handle("/places/", places)
	mux.Handle("/", server.Router())

	log.Printf("starting echotrails api on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
