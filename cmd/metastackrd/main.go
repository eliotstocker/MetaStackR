package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"metastackr/internal/db"
	"metastackr/internal/server"
	"metastackr/internal/vcs"
	"metastackr/internal/worker"
)

func main() {
	log.Println("Starting metastackrd orchestration daemon...")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:postgres@localhost:5432/metastackr?sslmode=disable"
	}

	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	ghToken := os.Getenv("GH_TOKEN")
	appID := os.Getenv("GITHUB_APP_ID")
	if appID == "" {
		appID = os.Getenv("APP_ID")
	}
	privateKeyPEM := os.Getenv("GITHUB_PRIVATE_KEY")
	if privateKeyPEM == "" {
		privateKeyPEM = os.Getenv("PRIVATE_KEY")
	}
	sqsQueueURL := os.Getenv("SQS_QUEUE_URL")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Connect(ctx, connStr)
	var repo *db.Repository
	if err != nil {
		log.Printf("[warning] Database connection failed: %v. Running in standalone mode.", err)
	} else {
		defer database.Close()
		log.Println("Running embedded database migrations...")
		if err := database.RunMigrations(ctx); err != nil {
			log.Printf("[warning] Migration execution note: %v", err)
		}
		repo = db.NewRepository(database)
	}

	ghClient, err := server.NewGitHubClientWithApp(appID, privateKeyPEM, ghToken)
	if err != nil {
		log.Printf("[warning] Failed to initialize GitHub App client: %v", err)
		ghClient = server.NewGitHubClient(ghToken)
	}
	if ghClient != nil && repo != nil {
		ghClient.SetRepository(repo)
	}
	engine := worker.NewEngine(repo, ghClient, nil)

	// If SQS is configured, start the SQS consumer loop
	if sqsQueueURL != "" && engine != nil {
		sqsWorker, err := worker.NewSQSWorker(engine, sqsQueueURL)
		if err == nil {
			go sqsWorker.Start(ctx)
		} else {
			log.Printf("[warning] Failed to initialize SQS worker: %v", err)
		}
	} else if repo != nil {
		// Fallback to local ticker loop
		go engine.StartReconciliationLoop(ctx, 10*time.Second)
	}

	srv := server.NewServer(repo, ghClient, webhookSecret, func(c context.Context, id uuid.UUID) error {
		return engine.ExecuteCascadeMerge(c, id)
	})
	engine.SetVCSResolver(func(c context.Context, repoFullName string) vcs.VCSProvider {
		return srv.VCSForRepo(c, repoFullName)
	})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// AWS Lambda Execution vs Standalone HTTP Daemon
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		log.Println("Executing in AWS Lambda mode")
		server.StartLambda(mux)
	} else {
		httpServer := &http.Server{
			Addr:    ":" + port,
			Handler: mux,
		}

		go func() {
			log.Printf("metastackrd HTTP server listening on port %s", port)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}()

		<-ctx.Done()
		log.Println("Shutting down metastackrd daemon gracefully...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("Server forced to shutdown: %v", err)
		}
		log.Println("Daemon exited cleanly.")
	}
}
