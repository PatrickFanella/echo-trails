package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/bootstrapadmin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var email, reason string
	flag.StringVar(&email, "email", "", "verified EchoTrails email account to promote")
	flag.StringVar(&reason, "reason", "initial_field_demo_bootstrap", "audit reason")
	flag.Parse()
	if email == "" {
		flag.Usage()
		os.Exit(2)
	}
	databaseURL := os.Getenv("ECHOTRAILS_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		log.Fatal("ECHOTRAILS_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	result, err := bootstrapadmin.PromoteInitialAdmin(ctx, pool, email, reason, time.Now().UTC())
	if err != nil {
		log.Fatal(err)
	}
	if result.AlreadyAdmin {
		fmt.Printf("initial admin already configured: %s\n", result.UserID)
		return
	}
	fmt.Printf("initial admin configured: %s\n", result.UserID)
}
