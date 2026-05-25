package main

import (
	"log"
	"net/http"
	"os"

	"echotrails/api/internal/api"
)

func main() {
	store := api.NewStore(api.DefaultPlaces())
	h := api.NewHandler(store)
	addr := ":8080"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}
