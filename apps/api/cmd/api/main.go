package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

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
	obs := api.NewObservability("api", slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	mux := http.NewServeMux()
	mux.HandleFunc("/health", obs.HandleHealth)
	mux.HandleFunc("/ready", obs.HandleReady)
	mux.HandleFunc("/metrics", obs.HandleMetrics)
	mux.Handle("/places/search", places)
	mux.Handle("/discovery/nearby", places)
	mux.Handle("/places/requests", places)
	mux.Handle("/places/", places)
	mux.Handle("/", server.Router())

	log.Printf("starting echotrails api on %s", addr)
	httpServer := &http.Server{Addr: addr, Handler: obs.Middleware(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
