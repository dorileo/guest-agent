//  Copyright 2023 Google Inc. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

// Package service is a package with os specific service handling logic.
package service

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/GoogleCloudPlatform/guest-logging-go/logger"
)

// ServiceState is the type used in the state mapping enum.
type ServiceState int

const (
	// serviceName is the string used to register with the service manager.
	serviceName = "google_guest_agent"
	// ServiceState is a mapping of a guest-agent known states (to be translated
	// to OS specific values).
	StateRunning ServiceState = iota
	StateStopped
)

// Manager defines the front interface between the main go routine and the
// OS specific implementation.
type Manager struct {
	// doneChan is the channel used to sync up with the main go routine.
	doneChan chan bool
	// handler is the OS specific implementation of serviceHandler interface.
	handler serviceHandler
}

// serviceHandler is the OS specific implementation interface.
type serviceHandler interface {
	// Register registers the application into the service manager. It will
	// perform diferent steps depending on the OS in question.
	Register(ctx context.Context) error
	// SetState changes the service state with the service manager.
	SetState(ctx context.Context, state ServiceState) error
}

// New initializes and allocates a service Manager instance.
func New() *Manager {
	doneChan := make(chan bool)
	sigChan := make(chan os.Signal, 1)

	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)
	go func() {
		sig := <-sigChan
		logger.Infof("GCE Guest Agent got signal: %d, leaving...", sig)
		close(sigChan)
		doneChan <- true
	}()

	return &Manager{
		doneChan: doneChan,
		handler:  newServiceHandler(doneChan),
	}
}

// Done exposes the done channel (doneChan) used to sync up with the
// main go routine. Mainly used for context cancelation and propagation.
func (mn *Manager) Done() <-chan bool {
	return mn.doneChan
}

// Register wraps the os specfic implementation for Register operation.
func (mn *Manager) Register(ctx context.Context) error {
	return mn.handler.Register(ctx)
}

// SetState wraps the OS specific implementation for SetState operation.
func (mn *Manager) SetState(ctx context.Context, state ServiceState) error {
	return mn.handler.SetState(ctx, state)
}
