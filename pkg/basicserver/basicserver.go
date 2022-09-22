// Package basicserver contains a dummy server implementation for testing metrics and logging
package basicserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	clarkezoneLog "github.com/jackfrancis/kustodian/pkg/log"
)

type cleanupfunc func()

// BasicServer object
type BasicServer struct {
	httpserver *http.Server
	ctx        context.Context
	cancel     context.CancelFunc
	exitchan   chan (bool)
	listenport int
}

// CreateBasicServer Create BasicServer object and return
func CreateBasicServer() *BasicServer {
	bs := BasicServer{}
	bs.listenport = 9090
	return &bs
}

// DefaultMux returns a mux preconfigured with defaults
func DefaultMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

// StartListen Start listening for a connection
func (bs *BasicServer) StartListen(mux http.Handler) {
	clarkezoneLog.Successf("starting... basic server on :%v", fmt.Sprint(bs.listenport))

	bs.exitchan = make(chan bool)
	bs.ctx, bs.cancel = context.WithCancel(context.Background())

	bs.httpserver = &http.Server{Addr: ":" + fmt.Sprint(bs.listenport)}

	if mux != nil {
		clarkezoneLog.Debugf("configuring mux")
		bs.httpserver.Handler = mux
	}

	go func() {
		err := bs.httpserver.ListenAndServe()
		if err.Error() != "http: Server closed" {
			panic(err)
		}
		defer func() {
			clarkezoneLog.Debugf("Webserver goroutine exited")
			bs.exitchan <- true
		}()
	}()
}

// WaitforInterupt Wait for a sigterm event or for user to press control c when running interacticely
func (bs *BasicServer) WaitforInterupt() error {
	if bs.exitchan == nil {
		clarkezoneLog.Debugf("WaitForInterupt(): server not started\n")
		return fmt.Errorf("server not started")
	}
	ch := make(chan struct{})
	handleSig(func() { close(ch) })
	clarkezoneLog.Successf("Waiting for user to press control c or sig terminate\n")
	<-ch
	clarkezoneLog.Debugf("Terminate signal detected, closing job manager\n")
	return bs.Shutdown()
}

func handleSig(cleanupwork cleanupfunc) chan struct{} {
	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan struct{})
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		clarkezoneLog.Debugf("\nhandleSig Received an interrupt, stopping services...\n")
		if cleanupwork != nil {
			clarkezoneLog.Debugf("cleanup work found")
			cleanupwork()
			clarkezoneLog.Debugf("cleanup work completed")
		}

		close(cleanupDone)
	}()
	return cleanupDone
}

// Shutdown terminates the listening thread
func (bs *BasicServer) Shutdown() error {
	if bs.exitchan == nil {
		clarkezoneLog.Debugf("BasicServer: no exit channel detected on shutdown\n")
		return fmt.Errorf("BasicServer: no exit channel detected on shutdown")
	}
	defer bs.ctx.Done()
	defer bs.cancel()
	clarkezoneLog.Debugf("BasicServer: request httpserver shutdown")
	httpexit := bs.httpserver.Shutdown(bs.ctx)
	clarkezoneLog.Debugf("BasicServer: shutdwon completed, wait for exitchan")
	<-bs.exitchan
	clarkezoneLog.Debugf("BasicServer: exit completed function returqn")
	return httpexit
}
