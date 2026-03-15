package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	api "github.com/thiemok/tiny-dash/api"
	"github.com/thiemok/tiny-dash/api/internal/dashboard"
	"github.com/thiemok/tiny-dash/api/internal/render"
)

func main() {
	chromePath := os.Getenv("CHROME_PATH")
	renderer, err := render.NewRenderer(chromePath)
	if err != nil {
		log.Fatalf("Failed to start renderer: %v", err)
	}
	defer renderer.Close()

	dashHandler, err := dashboard.NewDashboardHandler(api.TemplateFS)
	if err != nil {
		log.Fatalf("Failed to create dashboard handler: %v", err)
	}

	port := ":8080"
	baseURL := "http://localhost" + port

	imageHandler := dashboard.NewImageHandler(renderer, baseURL)

	mux := http.NewServeMux()
	dashHandler.RegisterRoutes(mux)
	imageHandler.RegisterRoutes(mux)

	mux.HandleFunc("GET /api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Hello, World!"))
	})

	mux.Handle("GET /static/", http.FileServerFS(api.StaticFS))

	log.Printf("Starting API server on %s", port)
	log.Printf("Endpoints:")
	log.Printf("  GET /dashboard                      — HTML dashboard")
	log.Printf("  GET /api/dashboard/image             — packed e-ink image")
	log.Printf("  GET /api/dashboard/preview           — dithered PNG preview")
	log.Printf("  GET /api/hello                       — health check")

	go func() {
		if err := http.ListenAndServe(port, mux); err != nil {
			log.Fatal(err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")
}
