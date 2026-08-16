package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/jobs"
	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/media"
	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/privacy"
	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/push"
	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/ranking"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mediaCatalog interface {
	media.Catalog
	media.TranscriptCatalog
	media.ModerationCatalog
	media.QuoteSnapshotCatalog
	media.TextModerationCatalog
	CleanupExpiredOriginals(context.Context, time.Time, int) (int, error)
}

type safetyServices interface {
	media.SafetyScanner
	media.TextSafetyScanner
}

type runtimeHandler struct {
	media   media.JobHandler
	push    *push.Dispatcher
	ranking *ranking.Engine
	privacy *privacy.Processor
}

func (h runtimeHandler) HandleJob(ctx context.Context, job jobs.Job) error {
	if job.Type == jobs.TypeSendPush {
		if h.push == nil {
			return errors.New("push dispatcher is not configured")
		}
		return h.push.Dispatch(ctx, job.AssetID)
	}
	if job.Type == jobs.TypeRankEcho {
		if h.ranking == nil {
			return errors.New("ranking engine is not configured")
		}
		return h.ranking.RecomputeEcho(ctx, job.AssetID)
	}
	if job.Type == jobs.TypePrivacyExport || job.Type == jobs.TypePrivacyDelete {
		if h.privacy == nil {
			return errors.New("privacy processor is not configured")
		}
		return h.privacy.Process(ctx, job.AssetID, job.Type)
	}
	return h.media.HandleJob(ctx, job)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	redisConfig := jobs.RedisConfig{URL: os.Getenv("ECHOTRAILS_REDIS_URL"), StreamName: os.Getenv("ECHOTRAILS_WORKER_STREAM"), GroupName: os.Getenv("ECHOTRAILS_WORKER_GROUP"), Consumer: os.Getenv("ECHOTRAILS_WORKER_CONSUMER")}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	queue, catalog, mediaPipeline, transcriber, scanner, pushDispatcher, rankingEngine, privacyProcessor, closeRuntime := runtimeDependencies(ctx, logger, redisConfig)
	defer closeRuntime()
	mediaHandler := media.JobHandler{
		Processor:      media.Processor{Catalog: catalog, Pipeline: mediaPipeline},
		Transcription:  media.TranscriptionProcessor{Catalog: catalog, Transcriber: transcriber},
		Moderation:     media.ModerationProcessor{Catalog: catalog, Scanner: scanner},
		Snapshots:      media.QuoteSnapshotProcessor{Catalog: catalog},
		TextModeration: media.TextModerationProcessor{Catalog: catalog, Scanner: scanner},
	}
	processor := jobs.Processor{Queue: queue, Logger: logger, Handler: runtimeHandler{media: mediaHandler, push: pushDispatcher, ranking: rankingEngine, privacy: privacyProcessor}}
	lastFullRanking := time.Time{}
	lastMediaCleanup := time.Time{}
	lastPrivacyCleanup := time.Time{}
	for {
		if rankingEngine != nil && time.Since(lastFullRanking) >= 5*time.Minute {
			if err := rankingEngine.RecomputeAll(ctx); err != nil && ctx.Err() == nil {
				logger.Error("recompute notability snapshots", "error", err)
			} else {
				lastFullRanking = time.Now()
			}
		}
		if time.Since(lastMediaCleanup) >= time.Hour {
			if cleaned, err := catalog.CleanupExpiredOriginals(ctx, time.Now().UTC(), 250); err != nil && ctx.Err() == nil {
				logger.Error("cleanup expired media originals", "error", err)
			} else {
				if cleaned > 0 {
					logger.Info("cleaned expired media originals", "count", cleaned)
				}
				lastMediaCleanup = time.Now()
			}
		}
		if privacyProcessor != nil && time.Since(lastPrivacyCleanup) >= time.Hour {
			if cleaned, err := privacyProcessor.CleanupExpiredExports(ctx, time.Now().UTC(), 250); err != nil && ctx.Err() == nil {
				logger.Error("cleanup expired privacy exports", "error", err)
			} else {
				if cleaned > 0 {
					logger.Info("cleaned expired privacy exports", "count", cleaned)
				}
				lastPrivacyCleanup = time.Now()
			}
		}
		if relay, ok := queue.(interface{ RelayDue(context.Context) error }); ok {
			if err := relay.RelayDue(ctx); err != nil && ctx.Err() == nil {
				logger.Error("relay Postgres jobs to Redis", "error", err)
			}
		}
		if err := processor.ProcessOne(ctx); err != nil {
			if errors.Is(err, jobs.ErrNoJobs) {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			if ctx.Err() != nil {
				return
			}
			logger.Error("worker process-one failed", "error", err)
		}
	}
}

func runtimeDependencies(ctx context.Context, logger *slog.Logger, redisConfig jobs.RedisConfig) (jobs.Queue, mediaCatalog, media.ProcessingPipeline, media.Transcriber, safetyServices, *push.Dispatcher, *ranking.Engine, *privacy.Processor, func()) {
	if !redisConfig.Valid() {
		logger.Error("complete Redis configuration is required")
		os.Exit(2)
	}
	queue, err := jobs.NewRedisQueue(ctx, redisConfig)
	if err != nil {
		logger.Error("configure Redis queue", "error", err)
		os.Exit(2)
	}
	databaseURL := os.Getenv("ECHOTRAILS_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		_ = queue.Close()
		logger.Error("ECHOTRAILS_DATABASE_URL is required")
		os.Exit(2)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		_ = queue.Close()
		logger.Error("configure Postgres", "error", err)
		os.Exit(2)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("connect Postgres", "error", err)
		os.Exit(2)
	}
	trackedQueue, err := jobs.NewPostgresTrackedQueue(queue, pool)
	if err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("configure tracked job queue", "error", err)
		os.Exit(2)
	}
	if err = trackedQueue.RequeueOrphans(ctx); err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("requeue orphaned jobs", "error", err)
		os.Exit(2)
	}
	pushEnabled := envBool("ECHOTRAILS_APNS_ENABLED", true)
	trackedQueue.SetPushDeliveryEnabled(pushEnabled)
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(requiredValue(logger, "ECHOTRAILS_S3_REGION")),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(requiredValue(logger, "ECHOTRAILS_S3_ACCESS_KEY_ID"), requiredValue(logger, "ECHOTRAILS_S3_SECRET_ACCESS_KEY"), "")),
	)
	if err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("configure object storage", "error", err)
		os.Exit(2)
	}
	s3Client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(requiredValue(logger, "ECHOTRAILS_S3_ENDPOINT"))
		options.UsePathStyle = true
	})
	objectWriter, err := media.NewS3ObjectWriter(s3Client)
	if err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("configure object writer", "error", err)
		os.Exit(2)
	}
	mediaPipeline, err := media.NewCommandPipeline(objectWriter, media.ExecToolRunner{},
		envOrDefault("ECHOTRAILS_FFMPEG_PATH", "ffmpeg"), envOrDefault("ECHOTRAILS_FFPROBE_PATH", "ffprobe"),
		envOrDefault("ECHOTRAILS_IMAGEMAGICK_PATH", "convert"), envOrDefault("ECHOTRAILS_MALWARE_SCANNER_PATH", "clamdscan"))
	if err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("configure media processing pipeline", "error", err)
		os.Exit(2)
	}
	mediaServices, err := media.NewHTTPMediaServices(requiredValue(logger, "ECHOTRAILS_MODERATION_SERVICE_URL"), requiredValue(logger, "ECHOTRAILS_MODERATION_SERVICE_TOKEN"), nil)
	if err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("configure moderation and transcription service", "error", err)
		os.Exit(2)
	}
	catalog, err := media.NewPostgresCatalog(pool, objectWriter, requiredValue(logger, "ECHOTRAILS_S3_BUCKET"))
	if err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("configure media catalog", "error", err)
		os.Exit(2)
	}
	var dispatcher *push.Dispatcher
	if pushEnabled {
		pushRepository, pushErr := push.NewPostgresRepository(pool, []byte(requiredValue(logger, "ECHOTRAILS_PUSH_TOKEN_ENCRYPTION_KEY")))
		if pushErr != nil {
			pool.Close()
			_ = queue.Close()
			logger.Error("configure push repository", "error", pushErr)
			os.Exit(2)
		}
		privateKey, readErr := os.ReadFile(requiredValue(logger, "ECHOTRAILS_APNS_PRIVATE_KEY_PATH"))
		if readErr != nil {
			pool.Close()
			_ = queue.Close()
			logger.Error("read APNs private key", "error", readErr)
			os.Exit(2)
		}
		apnsProvider, providerErr := push.NewAPNsProvider(push.APNsConfig{
			TeamID: requiredValue(logger, "ECHOTRAILS_APNS_TEAM_ID"), KeyID: requiredValue(logger, "ECHOTRAILS_APNS_KEY_ID"),
			BundleID: requiredValue(logger, "ECHOTRAILS_APNS_BUNDLE_ID"), PrivateKeyPEM: privateKey,
			SandboxEndpoint:    os.Getenv("ECHOTRAILS_APNS_SANDBOX_ENDPOINT"),
			ProductionEndpoint: os.Getenv("ECHOTRAILS_APNS_PRODUCTION_ENDPOINT"),
		})
		if providerErr != nil {
			pool.Close()
			_ = queue.Close()
			logger.Error("configure APNs provider", "error", providerErr)
			os.Exit(2)
		}
		dispatcher = &push.Dispatcher{Repository: pushRepository, Provider: apnsProvider}
	} else {
		logger.Info("APNs delivery disabled; notification jobs remain queued for Activity-first field testing")
	}
	rankingEngine, err := ranking.NewEngine(pool)
	if err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("configure ranking engine", "error", err)
		os.Exit(2)
	}
	privacyProcessor, err := privacy.NewProcessor(pool, objectWriter, requiredValue(logger, "ECHOTRAILS_S3_BUCKET"))
	if err != nil {
		pool.Close()
		_ = queue.Close()
		logger.Error("configure privacy processor", "error", err)
		os.Exit(2)
	}
	return trackedQueue, catalog, mediaPipeline, mediaServices, mediaServices, dispatcher, rankingEngine, privacyProcessor, func() { pool.Close(); _ = queue.Close() }
}

func requiredValue(logger *slog.Logger, name string) string {
	value := os.Getenv(name)
	if value == "" {
		logger.Error("required configuration missing", "name", name)
		os.Exit(2)
	}
	return value
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value != "0" && value != "false" && value != "FALSE" && value != "no" && value != "NO"
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
