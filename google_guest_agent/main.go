//  Copyright 2017 Google Inc. All Rights Reserved.
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

// GCEGuestAgent is the Google Compute Engine guest agent executable.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/guest-agent/google_guest_agent/events"
	mdsEvent "github.com/GoogleCloudPlatform/guest-agent/google_guest_agent/events/metadata"
	"github.com/GoogleCloudPlatform/guest-agent/google_guest_agent/events/sshtrustedca"
	"github.com/GoogleCloudPlatform/guest-agent/google_guest_agent/osinfo"
	"github.com/GoogleCloudPlatform/guest-agent/google_guest_agent/scheduler"
	"github.com/GoogleCloudPlatform/guest-agent/google_guest_agent/service"
	"github.com/GoogleCloudPlatform/guest-agent/google_guest_agent/sshca"
	"github.com/GoogleCloudPlatform/guest-agent/google_guest_agent/telemetry"
	"github.com/GoogleCloudPlatform/guest-agent/metadata"
	"github.com/GoogleCloudPlatform/guest-agent/utils"
	"github.com/GoogleCloudPlatform/guest-logging-go/logger"
	"github.com/go-ini/ini"
)

// Certificates wrapps a list of certificate authorities.
type Certificates struct {
	Certs []TrustedCert `json:"trustedCertificateAuthorities"`
}

// TrustedCert defines the object containing a public key.
type TrustedCert struct {
	PublicKey string `json:"publicKey"`
}

var (
	programName              = "GCEGuestAgent"
	version                  string
	oldMetadata, newMetadata *metadata.Descriptor
	config                   *ini.File
	osInfo                   osinfo.OSInfo
	mdsClient                *metadata.Client
)

const (
	winConfigPath = `C:\Program Files\Google\Compute Engine\instance_configs.cfg`
	configPath    = `/etc/default/instance_configs.cfg`
	regKeyBase    = `SOFTWARE\Google\ComputeEngine`
)

type manager interface {
	diff() bool
	disabled(string) bool
	set(ctx context.Context) error
	timeout() bool
}

func logStatus(name string, disabled bool) {
	var status string
	switch disabled {
	case false:
		status = "enabled"
	case true:
		status = "disabled"
	}
	logger.Infof("GCE %s manager status: %s", name, status)
}

func parseConfig(file string) (*ini.File, error) {
	// Priority: file.cfg, file.cfg.distro, file.cfg.template
	cfg, err := ini.LoadSources(ini.LoadOptions{Loose: true, Insensitive: true}, file, file+".distro", file+".template")
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func closeFile(c io.Closer) {
	err := c.Close()
	if err != nil {
		logger.Warningf("Error closing file: %v.", err)
	}
}

func runUpdate(ctx context.Context) {
	var wg sync.WaitGroup
	mgrs := []manager{&addressMgr{}}
	switch runtime.GOOS {
	case "windows":
		mgrs = append(mgrs, []manager{newWsfcManager(), &winAccountsMgr{}, &diagnosticsMgr{}}...)
	default:
		mgrs = append(mgrs, []manager{&clockskewMgr{}, &osloginMgr{}, &accountsMgr{}}...)
	}
	for _, mgr := range mgrs {
		wg.Add(1)
		go func(mgr manager) {
			defer wg.Done()
			if mgr.disabled(runtime.GOOS) {
				logger.Debugf("manager %#v disabled, skipping", mgr)
				return
			}
			if !mgr.timeout() && !mgr.diff() {
				logger.Debugf("manager %#v reports no diff", mgr)
				return
			}
			logger.Debugf("running %#v manager", mgr)
			if err := mgr.set(ctx); err != nil {
				logger.Errorf("error running %#v manager: %s", mgr, err)
			}
		}(mgr)
	}
	wg.Wait()
}

func runAgent(ctx context.Context, svc *service.Manager) error {
	opts := logger.LogOpts{LoggerName: programName}
	if runtime.GOOS == "windows" {
		opts.FormatFunction = logFormatWindows
		opts.Writers = []io.Writer{&utils.SerialPort{Port: "COM1"}}
	} else {
		opts.FormatFunction = logFormat
		opts.Writers = []io.Writer{os.Stdout}
		// Local logging is syslog; we will just use stdout in Linux.
		opts.DisableLocalLogging = true
	}

	if os.Getenv("GUEST_AGENT_DEBUG") != "" {
		opts.Debug = true
	}

	if err := logger.Init(ctx, opts); err != nil {
		return fmt.Errorf("error initializing logger: %v", err)
	}

	logger.Infof("GCE Agent Started (version %s)", version)

	osInfo = osinfo.Get()

	cfgfile := configPath
	if runtime.GOOS == "windows" {
		cfgfile = winConfigPath
	}

	var err error
	config, err = parseConfig(cfgfile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error parsing config %s: %s", cfgfile, err)
	}

	mdsClient = metadata.New()

	agentInit(ctx)

	if err := svc.SetState(ctx, service.StateRunning); err != nil {
		return fmt.Errorf("failed to set service state to running: %+v", err)
	}

	// Previous request to metadata *may* not have worked becasue routes don't get added until agentInit.
	if newMetadata == nil {
		/// Error here doesn't matter, if we cant get metadata, we cant record telemetry.
		newMetadata, err = mdsClient.Get(ctx)
		if err != nil {
			logger.Debugf("Error getting metdata: %v", err)
		}
	}

	// Try to re-initialize logger now, we know after agentInit() is more likely to have metadata available.
	// TODO: move all this metadata dependent code to its own metadata event handler.
	if newMetadata != nil {
		opts.ProjectName = newMetadata.Project.ProjectID
		if err := logger.Init(ctx, opts); err != nil {
			logger.Errorf("Error initializing logger: %v", err)
		}
	}

	// knownJobs is list of default jobs that run on a pre-defined schedule.
	knownJobs := []scheduler.Job{telemetry.New(mdsClient, programName, version)}
	scheduler.ScheduleJobs(ctx, knownJobs, false)

	eventsConfig := &events.Config{
		Watchers: []string{
			mdsEvent.WatcherID,
		},
	}

	// Only Enable sshtrustedca Watcher if osLogin is enabled.
	// TODO: ideally we should have a feature flag specifically for this.
	osLoginEnabled, _, _ := getOSLoginEnabled(newMetadata)
	if osLoginEnabled {
		eventsConfig.Watchers = append(eventsConfig.Watchers, sshtrustedca.WatcherID)
	}

	eventManager, err := events.New(eventsConfig)
	if err != nil {
		return fmt.Errorf("error initializing event manager: %v", err)
	}

	sshca.Init(eventManager)

	oldMetadata = &metadata.Descriptor{}
	eventManager.Subscribe(mdsEvent.LongpollEvent, nil, func(ctx context.Context, evType string, data interface{}, evData *events.EventData) bool {
		logger.Debugf("Handling metadata %q event.", evType)

		// If metadata watcher failed there isn't much we can do, just ignore the event and
		// allow the water to get it corrected.
		if evData.Error != nil {
			logger.Infof("Metadata event watcher failed, ignoring: %+v", evData.Error)
			return true
		}

		if evData.Data == nil {
			logger.Infof("Metadata event watcher didn't pass in the metadata, ignoring.")
			return true
		}

		newMetadata = evData.Data.(*metadata.Descriptor)
		runUpdate(ctx)
		oldMetadata = newMetadata

		return true
	})

	eventManager.Run(ctx)
	logger.Infof("GCE Agent Stopped")
	return nil
}

func logFormatWindows(e logger.LogEntry) string {
	now := time.Now().Format("2006/01/02 15:04:05")
	// 2006/01/02 15:04:05 GCEGuestAgent This is a log message.
	return fmt.Sprintf("%s %s: %s", now, programName, e.Message)
}

func logFormat(e logger.LogEntry) string {
	switch e.Severity {
	case logger.Error, logger.Critical, logger.Debug:
		// ERROR file.go:82 This is a log message.
		return fmt.Sprintf("%s %s:%d %s", strings.ToUpper(e.Severity.String()), e.Source.File, e.Source.Line, e.Message)
	default:
		// This is a log message.
		return e.Message
	}
}

func closer(c io.Closer) {
	err := c.Close()
	if err != nil {
		logger.Warningf("Error closing %v: %v.", c, err)
	}
}

func main() {
	ctx, cancelContext := context.WithCancel(context.Background())

	svc := service.New()
	go func() {
		<-svc.Done()

		if err := svc.SetState(ctx, service.StateStopped); err != nil {
			logger.Fatalf("Failed to set service state to StopPending: %+v", err)
		}

		cancelContext()
	}()

	if err := svc.Register(ctx); err != nil {
		logger.Fatalf("Could not register into system's service manager: %+v", err)
	}

	if err := runAgent(ctx, svc); err != nil {
		logger.Fatalf("Failed to run agent: %+v", err)
	}
}
