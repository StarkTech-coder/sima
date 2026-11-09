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
	address := flag.String("http", "127.0.0.1:8080", "loopback HTTP address")
	flag.Parse()
	host, _, err := net.SplitHostPort(*address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("HTTP address must use a loopback IP, e.g. 127.0.0.1:8080")
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	sim, err := app.New(ctx)
	if err != nil {
		return err
	}
	defer sim.Close()
	server := &http.Server{Handler: web.Handler(sim), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Printf("\nS.I.M.A. — LOCAL SIMULATION\nDashboard: http://%s\nAEGIS UDP • 3 AUV • no hardware / sonar / acoustic modem\n\n", listener.Addr())
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
	return sim.Close()
}
