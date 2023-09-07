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

package service

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/guest-logging-go/logger"
	"golang.org/x/sys/windows/svc"
)

// winService is the serviceHandler interface implementation for windows.
type winService struct {
	// doneChan is the communication channel between the main go routine and the services.
	doneChan chan bool
	// statusChannel the windows service status reporting channel.
	statusChannel chan<- svc.Status
	// initialized is a guardrail flag determining if the windows service implementation is initialized.
	initialized bool
}

// newServiceHandler initializes the windows service handler.
func newServiceHandler(doneChan chan bool) serviceHandler {
	return &winService{
		doneChan:    doneChan,
		initialized: false,
	}
}

// handleInterrogate handles windows service's interrogate request. If returns true if needed to
// renew and false otherwise.
func (wh *winService) handleInterrogate(request <-chan svc.ChangeRequest) bool {
	select {
	case <-wh.doneChan:
		return false // should not renew
	case req := <-request:
		switch req.Cmd {
		case svc.Interrogate:
			logger.Debugf("Got an interrogate request from service manager, reporting status: %d.", req.CurrentStatus)
			wh.statusChannel <- req.CurrentStatus
			return true // should renew
		}
	default:
		return false // should not renew - we maybe got a shutdown/stop request
	}
	return false
}

// handle termination request from windows service manager and the doneChan used by main
// goroutine to communicate we are about to leave (i.e. after getting a SIGTERM signal). It
// returns true if needed to renew and false otherwise.
func (wh *winService) handleTermination(request <-chan svc.ChangeRequest) bool {
	select {
	case <-wh.doneChan:
		logger.Debugf("Got a done signal via doneChan.")
		return false // should not renew - we are leaving
	case req := <-request:
		switch req.Cmd {
		case svc.Stop, svc.Shutdown:
			logger.Debugf("Got a stop or shutdown signal from windows service manager.")
			wh.doneChan <- true
			return false // should not renew - we are leaving
		default:
			return true // should renew, we got non shutdown/stop request
		}
	}
}

// Execute is the windows service library interface implementation, it encapsulates status
// and requests handling. As per the library documentation: "the service will exit once Execute() completes".
func (wh *winService) Execute(_ []string, request <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	logger.Infof("Setting windows service status to: StartPending.")
	wh.statusChannel = status
	status <- svc.Status{State: svc.StartPending}
	go func() {
		for renew := true; renew; {
			renew = wh.handleInterrogate(request)
		}
	}()
	go func() {
		for renew := true; renew; {
			renew = wh.handleTermination(request)
		}
	}()
	return false, 0
}

// Register is the implementation of serviceHandler interface. It registers the application as a service
// in the windows service manager.
func (wh *winService) Register(ctx context.Context) error {
	if wh.initialized {
		return nil
	}

	logger.Debugf("Registering service with windows service manager.")
	if err := svc.Run(serviceName, wh); err != nil {
		return fmt.Errorf("failed to register windows service: %+v", err)
	}

	wh.initialized = true
	return nil
}

// SetState changes the state of the service with the service manager.
func (wh *winService) SetState(ctx context.Context, state ServiceState) error {
	if state == StateRunning {
		wh.statusChannel <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	} else if state == StateStopped {
		wh.statusChannel <- svc.Status{State: svc.StopPending}
	} else {
		return fmt.Errorf("unknown service state: %d", state)
	}
}
