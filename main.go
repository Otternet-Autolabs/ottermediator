package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/i574789/ottermediator/api"
	"github.com/i574789/ottermediator/chromecast"
	"github.com/i574789/ottermediator/config"
)

//go:embed frontend
var frontendFS embed.FS

func main() {
	port := flag.Int("port", 0, "Port to listen on (default 8006, overrides PORT env var)")
	cfgPath := flag.String("config", "ottermediator.json", "Path to config file")
	flag.Parse()

	// Port resolution: flag > env > default
	listenPort := 8006
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", &listenPort)
	}
	if *port != 0 {
		listenPort = *port
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Create WebSocket hub
	var dm *chromecast.DiscoveryManager
	hub := api.NewHub(func() []byte {
		if dm == nil {
			return nil
		}
		return api.DevicesMessage(dm.AllStatuses())
	})
	go hub.Run()

	// Create discovery manager and start scanning
	dm = chromecast.NewDiscoveryManager(cfg, hub)
	go dm.Start(ctx)

	// HTTP mux
	mux := http.NewServeMux()
	handler := api.NewHandler(dm, cfg)
	handler.RegisterRoutes(mux, hub)

	// Serve frontend static files
	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("frontend embed: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	addr := fmt.Sprintf(":%d", listenPort)
	log.Printf("ottermediator listening on http://localhost%s", addr)

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		srv.Shutdown(context.Background())
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
