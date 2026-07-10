package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/api"
	appleauth "git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/auth/apple"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	addr := os.Getenv("ECHOTRAILS_API_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	store, profiles, placeStore, echoStore, authorizationStore, mediaStore, socialStore, engagementStore, closeStore := configuredStores()
	defer closeStore()
	var appleService *appleauth.Service
	if identityStore, ok := store.(appleauth.IdentityStore); ok {
		verifier, err := appleauth.NewProductionVerifier(requiredValue("ECHOTRAILS_APPLE_CLIENT_ID"), nil)
		if err != nil {
			log.Fatalf("configure Sign in with Apple: %v", err)
		}
		appleService = appleauth.NewService(verifier, identityStore)
	}
	emailSender := configuredEmailSender()
	server := api.NewServer(api.Config{Store: store, Profiles: profiles, Sender: emailSender, CodePepper: requiredSecret("ECHOTRAILS_AUTH_CODE_PEPPER"), Apple: appleService})
	discoveryStore, _ := placeStore.(api.DiscoveryRepository)
	accountStore, _ := placeStore.(api.AccountRepository)
	adminStore, _ := placeStore.(api.AdminRepository)
	appealStore, _ := placeStore.(api.AppealRepository)
	places := api.NewHandlerWithConfig(placeStore, api.HandlerConfig{
		Media:              mediaStore,
		Profiles:           profiles,
		Echoes:             echoStore,
		Authorizations:     authorizationStore,
		Social:             socialStore,
		Engagement:         engagementStore,
		Discovery:          discoveryStore,
		Account:            accountStore,
		Admin:              adminStore,
		Appeals:            appealStore,
		CreationSigningKey: requiredSecret("ECHOTRAILS_CREATION_TOKEN_SIGNING_KEY"),
		PublicLocationKey:  requiredSecret("ECHOTRAILS_PUBLIC_LOCATION_KEY"),
	})
	obs := api.NewObservability("api", slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	mux := http.NewServeMux()
	mux.HandleFunc("/health", obs.HandleHealth)
	mux.HandleFunc("/ready", obs.HandleReady)
	mux.HandleFunc("/metrics", obs.HandleMetrics)
	mux.Handle("/places/search", places)
	mux.Handle("/discovery/nearby", places)
	mux.Handle("/discovery/notable", places)
	mux.Handle("/search", places)
	mux.Handle("/media/uploads", places)
	mux.Handle("/media/uploads/", places)
	mux.Handle("/media/assets/", places)
	mux.Handle("/me/avatar", places)
	mux.Handle("/me/avatar/uploads", places)
	mux.Handle("/places/requests", places)
	mux.Handle("/places/", places)
	mux.Handle("/creation-tokens", places)
	mux.Handle("/beta/access", places)
	mux.Handle("/consent/", places)
	mux.Handle("/privacy/", places)
	mux.Handle("/analytics/", places)
	mux.Handle("/account/sessions", places)
	mux.Handle("/account/sessions/", places)
	mux.Handle("/account/bootstrap", places)
	mux.Handle("/admin/", places)
	mux.Handle("/appeals", places)
	mux.Handle("/appeals/", places)
	mux.Handle("/echoes", places)
	mux.Handle("/echoes/", places)
	mux.Handle("/reports", places)
	mux.Handle("/hides", places)
	mux.Handle("/blocks", places)
	mux.Handle("/notifications", places)
	mux.Handle("/subscriptions", places)
	mux.Handle("/devices", places)
	mux.Handle("/devices/", places)
	mux.Handle("/notification-preferences", places)
	mux.Handle("/mutes/", places)
	mux.Handle("/", server.Router())

	log.Printf("starting echotrails api on %s", addr)
	httpServer := &http.Server{Addr: addr, Handler: obs.Middleware(api.BearerAuthMiddleware(store, api.ReleaseAccessMiddleware(accountStore, mux))), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func configuredStores() (api.Store, api.ProfileStore, api.PlaceStore, api.EchoStore, api.CreationAuthorizationStore, api.MediaUploadStore, api.SocialRepository, api.EngagementRepository, func()) {
	databaseURL := os.Getenv("ECHOTRAILS_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		log.Fatal("ECHOTRAILS_DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("configure postgres: %v", err)
	}
	if err = pool.Ping(context.Background()); err != nil {
		pool.Close()
		log.Fatalf("connect postgres: %v", err)
	}
	postgresStore, err := api.NewPostgresStore(pool, requiredSecret("ECHOTRAILS_EXACT_LOCATION_ENCRYPTION_KEY"), requiredSecret("ECHOTRAILS_PUSH_TOKEN_ENCRYPTION_KEY"))
	if err != nil {
		pool.Close()
		log.Fatalf("configure postgres repositories: %v", err)
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(requiredValue("ECHOTRAILS_S3_REGION")),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(requiredValue("ECHOTRAILS_S3_ACCESS_KEY_ID"), requiredValue("ECHOTRAILS_S3_SECRET_ACCESS_KEY"), "")),
	)
	if err != nil {
		pool.Close()
		log.Fatalf("configure object storage: %v", err)
	}
	s3Client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(requiredValue("ECHOTRAILS_S3_ENDPOINT"))
		options.UsePathStyle = true
	})
	objectStorage, err := api.NewS3ObjectStorage(s3Client)
	if err != nil {
		pool.Close()
		log.Fatalf("configure object storage: %v", err)
	}
	mediaStore, err := api.NewPostgresMediaUploadStore(pool, objectStorage, requiredValue("ECHOTRAILS_S3_BUCKET"))
	if err != nil {
		pool.Close()
		log.Fatalf("configure media repository: %v", err)
	}
	return postgresStore, postgresStore, postgresStore, postgresStore, postgresStore, mediaStore, postgresStore, postgresStore, pool.Close
}

func requiredSecret(name string) []byte {
	value := []byte(os.Getenv(name))
	if len(value) < 32 {
		log.Fatalf("%s must contain at least 32 bytes", name)
	}
	return value
}

func requiredValue(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func configuredEmailSender() api.EmailSender {
	sender, err := api.NewSMTPSender(api.SMTPConfig{
		Host: requiredValue("ECHOTRAILS_SMTP_HOST"), Port: requiredValue("ECHOTRAILS_SMTP_PORT"),
		Username: os.Getenv("ECHOTRAILS_SMTP_USERNAME"), Password: os.Getenv("ECHOTRAILS_SMTP_PASSWORD"), From: requiredValue("ECHOTRAILS_SMTP_FROM"),
		AuthLinkBaseURL: requiredValue("ECHOTRAILS_AUTH_LINK_BASE_URL"),
		AllowInsecure:   os.Getenv("ECHOTRAILS_SMTP_ALLOW_INSECURE") == "1" && os.Getenv("ECHOTRAILS_ENV") != "production",
	})
	if err != nil {
		log.Fatalf("configure SMTP: %v", err)
	}
	return sender
}
