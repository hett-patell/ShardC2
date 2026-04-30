package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/server"
)

const banner = `
  ____  _                   _  ____ ____  
 / ___|| |__   __ _ _ __ __| |/ ___|___ \ 
 \___ \| '_ \ / _' | '__/ _' | |     __) |
  ___) | | | | (_| | | | (_| | |___ / __/ 
 |____/|_| |_|\__,_|_|  \__,_|\____|_____|

 Command & Control Framework v0.1.0
`

func main() {
	var (
		addr    = flag.String("addr", ":8443", "Server listen address")
		dbConn  = flag.String("db", "postgres://shardc2:shardc2_secret@localhost:5432/shardc2?sslmode=disable", "Database connection string")
		migrate = flag.Bool("migrate", false, "Run database migrations on startup")
	)
	flag.Parse()

	fmt.Print(banner)
	fmt.Printf("[*] PID: %d\n", os.Getpid())

	db, err := database.New(*dbConn)
	if err != nil {
		log.Fatalf("[-] Database connection failed: %v", err)
	}
	defer db.Close()
	fmt.Println("[+] Database connected")

	if *migrate {
		if err := db.RunMigrations("migrations"); err != nil {
			log.Fatalf("[-] Migration failed: %v", err)
		}
		fmt.Println("[+] Migrations applied")
	}

	srv := server.New(db)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		fmt.Println("\n[*] Shutting down...")
		srv.Shutdown()
	}()

	if err := srv.Start(*addr); err != nil {
		log.Fatalf("[-] Server error: %v", err)
	}
}
