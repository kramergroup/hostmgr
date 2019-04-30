package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/apsdehal/go-logger"
	"github.com/giantswarm/retry-go"
	"github.com/kramergroup/hostmgr/pkg/app"
)

var redisURL = flag.String("host", "localhost:6379", "Redis server url")
var baseFilter = flag.String("filter", "/hostmgr", "Filter for key events")
var consumerMode = flag.Bool("server", false, "Server mode manages this host")
var announcerMode = flag.Bool("client", false, "Client mode announces this host to participating servers")
var clientUser = flag.String("user", "", "The username of the ssh client executer (not the login name)")
var debug = flag.Bool("debug", false, "Show debugging output")

var log, _ = logger.New("main", 3, os.Stdout)

func main() {

	// Parse the command-line flags
	flag.Parse()

	if !*announcerMode && !*consumerMode {
		log.Error("No mode selected. Terminating")
		return
	}

	/*
		Initialise a channel that counts the number of running modes.
	*/
	termCh := make(chan bool)
	nTasks := 0

	/*
		Use consumerMode as default if no mode is defined
	*/
	if !*announcerMode && !*consumerMode {
		*consumerMode = true
	}

	// Listen for clients
	if *consumerMode {

		updateOp := func() error {
			q := app.NewQuery(*redisURL, *baseFilter)
			defs, err := q.HostDefinitions()
			if err != nil {
				log.WarningF("Error during querying host definitions. [%s]", err.Error())
				return err
			}
			return app.UpdateSSHConfiguration(defs)
		}

		watchOp := func() error {
			watcher := app.NewWatcher(*redisURL, *baseFilter, updateOp)
			defer watcher.Stop()

			err := watcher.Start()
			return err
		}

		go func() {
			log.InfoF("Starting to listen for udates from %s", *redisURL)
			nTasks = nTasks + 1
			retry.Do(watchOp, retry.Sleep(2*time.Second), retry.Timeout(60*time.Second))
			log.Info("Stopped listening")
			termCh <- true
		}()

		/*
			Update the ssh configuration before start watching
			for changes. This will catch changes that were
			missed while the instance was offline.
		*/
		retry.Do(updateOp, retry.Sleep(2*time.Second), retry.Timeout(60*time.Second))
	}

	// Announce to the world
	if *announcerMode {
		go func() {
			nTasks = nTasks + 1
			def := app.Create()
			// Override the client user if specified on command line
			if *clientUser != "" {
				def.ClientUser = *clientUser
			}

			log.InfoF("Announceing host to %s", *redisURL)

			var key string
			announceOp := func() error {
				var err error
				q := app.NewQuery(*redisURL, *baseFilter)
				key, err = q.Announce(def)
				return err
			}

			err := retry.Do(announceOp, retry.Sleep(5*time.Second), retry.Timeout(60*time.Second))

			if err != nil {
				log.Error(fmt.Sprintf("Could not announce host [%s]", err.Error()))
			} else {
				/*
					We wait for system signals and attempt to die gracefully by
					revoking the host definition
				*/
				c := make(chan os.Signal)
				signal.Notify(c, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGHUP, syscall.SIGQUIT)

				select {
				case <-c:
					log.Info("Revoking host announcement")
					q := app.NewQuery(*redisURL, *baseFilter)
					q.Revoke(key)
				}
			}
			termCh <- true
		}()
	}

	// Wait for all processes to finish
	for {
		select {
		case <-termCh:
			nTasks = nTasks - 1
		}
		if nTasks <= 0 {
			return
		}
	}

}
