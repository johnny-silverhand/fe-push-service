package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"./server"
)

var flagConfigFile string

var stopChan chan os.Signal = make(chan os.Signal)

func main() {

	flag.StringVar(&flagConfigFile, "config", "push-proxy.json", "")
	flag.Parse()
	server.LoadConfig(flagConfigFile)

	server.Start()

	// wait for kill signal before attempting to gracefully shutdown
	// the running service
	signal.Notify(stopChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-stopChan

	server.Stop()
}
