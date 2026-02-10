package main

import (
	"flag"
	"fmt"
	"os"

	"srv.exe.dev/srv"
)

var (
	flagListenAddr  = flag.String("listen", ":8000", "address to listen on")
	flagAdminPass   = flag.String("admin-password", "", "admin panel password (or ADMIN_PASSWORD env var)")
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func run() error {
	flag.Parse()
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	adminPass := *flagAdminPass
	if adminPass == "" {
		adminPass = os.Getenv("ADMIN_PASSWORD")
	}
	server, err := srv.New("db.sqlite3", hostname, adminPass)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	return server.Serve(*flagListenAddr)
}
