// Copyright 2014 Salsita s.r.o.
//
// This file is part of salsita-ci-heroku.
//
// salsita-ci-heroku is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// salsita-ci-heroku is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with salsita-ci-heroku.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/cider/cider/broker"
	"github.com/cider/cider/broker/exchanges/rpc/roundrobin"
	"github.com/cider/cider/broker/transports/websocket/rpc"

	"code.google.com/p/go.net/websocket"
)

const TokenHeader = "X-SalsitaCI-Token"

var (
	port  = mustGetenv("PORT")
	token = mustGetenv("TOKEN")
)

func main() {
	if err := innerMain(); err != nil {
		panic(err)
	}
}

func innerMain() error {
	// Start catching signals.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Prepare the RPC service exchange.
	balancer := roundrobin.NewBalancer()

	// Set up the RPC service endpoint using WebSocket as the transport.
	broker.RegisterEndpointFactory("ws_rpc", func() (broker.Endpoint, error) {
		factory := rpc.EndpointFactory{
			Addr: ":" + port,
			WSHandshake: func(cfg *websocket.Config, req *http.Request) error {
				// Make sure that the access token is present in the request.
				tokenHeader := req.Header.Get(TokenHeader)
				// Since token is never empty, we do not have to handle that case.
				if tokenHeader != token {
					return ErrInvalidToken
				}
				return nil
			},
		}

		return factory.NewEndpoint(balancer)
	})

	// Register a broker monitoring channel.
	monitorCh := make(chan *broker.EndpointCrashReport)
	broker.Monitor(monitorCh)

	// Create the registered service endpoints.
	broker.ListenAndServe()
	defer func() {
		log.Println("Waiting for the broker to terminate...")
		broker.Terminate()
		log.Println("Broker terminated")
	}()

	// Block until the endpoint crashes or a signal is received.
Loop:
	for {
		select {
		case err, ok := <-monitorCh:
			if !ok {
				break Loop
			}
			log.Printf("Endpoint %v crashed: %v\n", err.FactoryId, err.Error)
			if err.Dropped {
				err := fmt.Errorf("Endpoint %v dropped", err.FactoryId)
				log.Println(err)
				return err
			}
		case <-signalCh:
			log.Println("Signal received, terminating ...")
			break Loop
		}
	}

	return nil
}

func mustGetenv(key string) (value string) {
	value = os.Getenv(key)
	if value == "" {
		panic(fmt.Errorf("Environment variable %s is not set", key))
	}
}
