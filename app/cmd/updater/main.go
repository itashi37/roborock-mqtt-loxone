package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mqtt-home/roborock-mqtt/updater"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--healthcheck" {
		client := http.Client{Timeout: 5 * time.Second}
		response, err := client.Get(os.Args[2])
		if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
			os.Exit(1)
		}
		_ = response.Body.Close()
		return
	}
	service, err := updater.NewService(updater.Dependencies{
		Engine:        updater.NewDockerEngine(environment("DOCKER_SOCKET", "/var/run/docker.sock")),
		ContainerName: environment("BRIDGE_CONTAINER", "roborock-mqtt-loxone"),
		HealthURL:     environment("BRIDGE_STATUS_URL", "http://roborock-mqtt-loxone:8080/api/system/status"),
		DataDir:       environment("BRIDGE_DATA_DIR", "/bridge-data"),
		MinimumFree:   uint64(environmentInt("UPDATER_MIN_FREE_MB", 512)) * 1024 * 1024,
		HealthTimeout: time.Duration(environmentInt("UPDATER_HEALTH_TIMEOUT_SECONDS", 180)) * time.Second,
	})
	if err != nil {
		slog.Error("failed to initialize updater", "error", err)
		os.Exit(1)
	}
	server, err := updater.NewServer(service, readSecret(), environmentInt("UPDATER_RATE_LIMIT_PER_HOUR", 3), time.Hour)
	if err != nil {
		slog.Error("failed to initialize updater API", "error", err)
		os.Exit(1)
	}
	address := environment("UPDATER_LISTEN", ":8090")
	slog.Info("isolated updater ready", "address", address, "image_allowlist", updater.AllowedImage)
	httpServer := &http.Server{Addr: address, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("updater server stopped", "error", err)
		os.Exit(1)
	}
}

func readSecret() string {
	if path := os.Getenv("UPDATER_TOKEN_FILE"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
	}
	return os.Getenv("UPDATER_TOKEN")
}

func environment(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func environmentInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
