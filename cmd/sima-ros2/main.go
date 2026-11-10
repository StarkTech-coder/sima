package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/StarkTech-coder/sima/internal/app"
	"github.com/StarkTech-coder/sima/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	httpAddress := flag.String("http", "127.0.0.1:8081", "loopback HTTP address")
	bridge := flag.String("bridge", "http://127.0.0.1:8090", "ROS2 bridge URL")
	vehicles := flag.String("vehicles", "AUV-01,AUV-02,AUV-03", "comma-separated vehicle IDs")
	flag.Parse()
	host, _, err := net.SplitHostPort(*httpAddress)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("HTTP address must use a loopback IP, e.g. 127.0.0.1:8081")
	}
	listener, err := net.Listen("tcp", *httpAddress)
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ids := make([]string, 0, 8)
	for _, part := range strings.Split(*vehicles, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			ids = append(ids, part)
		}
	}
	ros, err := app.NewROS2(ctx, app.ROS2Config{BridgeURL: *bridge, VehicleIDs: ids})
	if err != nil {
		return err
	}
	defer ros.Close()
	server := &http.Server{Handler: web.Handler(ros), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Printf("\nS.I.M.A. — ROS2 MODE\nDashboard: http://%s\nBridge: %s\nVehicles: %s\n\n", listener.Addr(), *bridge, strings.Join(ids, ","))
	select {
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err = server.Shutdown(shutdown); err != nil {
		_ = server.Close()
		return err
	}
	return ros.Close()
}
