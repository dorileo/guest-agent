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

//go:build !windows

package service

import (
	"context"
	"fmt"
	"os"

	"github.com/GoogleCloudPlatform/guest-agent/google_guest_agent/run"
	"github.com/GoogleCloudPlatform/guest-logging-go/logger"
)

// systemdService is the serviceHandler interface implementatin for systemd.
type systemdService struct {
	// doneChan is the communication channel between the main go routine and the service.
	doneChan chan bool
	// systemdContext determines if we were launched by systemd.
	systemdContext bool
}

// newServiceHandler initializes the systemd's service handler.
func newServiceHandler(doneChan chan bool) serviceHandler {
	return &systemdService{
		doneChan:       doneChan,
		systemdContext: os.Getenv("NOTIFY_SOCKET") != "",
	}
}

// Register is the implementation of serviceHandler interface. On systemd we are only
// sending a notify with an arbitrary status string.
func (ss *systemdService) Register(ctx context.Context) error {
	// Don't do anything if we are not running in a systemd context.
	if !ss.systemdContext {
		return nil
	}

	logger.Debugf("Registering service with systemd service manager.")
	return run.Quiet(ctx, "systemd-notify", "--status='Initializing service...'")
}

// SetState changes the state of the service with the service manager. For StateRunning
// we send a systemd-notify with READY=1 and an arbitrary string on STATUS, for
// StateStopped we are sending STOPPING=1 and an abritrary string on STATUS.
func (ss *systemdService) SetState(ctx context.Context, state ServiceState) error {
	// Don'tdo anything if we are not running in a systemd context.
	if !ss.systemdContext {
		return nil
	}

	if state == StateRunning {
		return run.Quiet(ctx, "systemd-notify", "--ready", "--status='Running service...'")
	} else if state == StateStopped {
		return run.Quiet(ctx, "systemd-notify", "--status='Stopping service...'")
	} else {
		return fmt.Errorf("unknown service state: %d", state)
	}
}
