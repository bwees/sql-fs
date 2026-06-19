package main

import (
	"flag"
	"fuse-demo/internal"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}
	mountPoint := flag.Arg(0)

	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		log.Fatalf("Unable to create mountpoint %s: %v", mountPoint, err)
	}

	fs := internal.NewSqlFS()

	server, err := fuse.NewServer(
		fs,
		"/tmp/mnt", // mount point (must exist)
		&fuse.MountOptions{
			Debug: true,
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	go server.Serve()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Received %s, unmounting %s", sig, mountPoint)
		if err := server.Unmount(); err != nil {
			log.Printf("Unmount error: %v", err)
		}
		os.Exit(0)
	}()

	server.Wait()
}
