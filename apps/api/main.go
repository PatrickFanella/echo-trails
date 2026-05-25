package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	store := NewStore()
	server := &http.Server{
		Addr:    ":8080",
		Handler: NewHandler(store),
	}
	log.Printf("listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Println(err)
		os.Exit(1)
	}
}
