//  Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
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

package app

import (
	"context"
	"os"

	"github.com/nvidia/nvsentinel/cmd/device-apiserver/app/options"
	_ "github.com/nvidia/nvsentinel/pkg/services/device/v1alpha1"
	"github.com/nvidia/nvsentinel/pkg/util/verflag"
	utilversion "github.com/nvidia/nvsentinel/pkg/util/version"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/term"
	"k8s.io/klog/v2"
)

// NewAPIServerCommand creates a *cobra.Command object with default parameters
func NewAPIServerCommand() *cobra.Command {
	s := options.NewServerRunOptions()
	ctx := genericapiserver.SetupSignalContext()

	cmd := &cobra.Command{
		Use: "device-apiserver",
		Long: `The Device API server validates and configures data
for the api objects which include gpus and others. The API Server services
gRPC operations and provides the frontend to a node's shared state through
which all other node-local components interact.`,

		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()

			fs := cmd.Flags()
			// Activate logging as soon as possible, after that
			// show flags with the final logging configuration.
			logsapi.ReapplyHandling = logsapi.ReapplyHandlingIgnoreUnchanged
			if err := logsapi.ValidateAndApply(s.Logs, nil); err != nil {
				return err
			}

			cliflag.PrintFlags(fs)

			// set default options
			completedOptions, err := s.Complete(ctx)
			if err != nil {
				return err
			}

			// validate options
			if errs := completedOptions.Validate(); len(errs) != 0 {
				return utilerrors.NewAggregate(errs)
			}

			return Run(ctx, completedOptions)
		},
		Args: cobra.NoArgs,
	}
	cmd.SetContext(ctx)

	fs := cmd.Flags()
	namedFlagSets := s.Flags()
	verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name(), logs.SkipLoggingConfigurationFlags())

	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cliflag.SetUsageAndHelpFunc(cmd, namedFlagSets, cols)

	return cmd
}

// Run runs the specified APIServer. This should never exit.
func Run(ctx context.Context, opts options.CompletedOptions) error {
	logger := klog.FromContext(ctx).WithValues("node", opts.NodeName)
	ctx = klog.NewContext(ctx, logger)

	logger.Info("Initializing Device API Server", "version", utilversion.Get())
	logger.V(2).Info("Golang settings",
		"GOGC", os.Getenv("GOGC"),
		"GOMAXPROCS", os.Getenv("GOMAXPROCS"),
		"GOTRACEBACK", os.Getenv("GOTRACEBACK"),
	)

	config, err := NewConfig(ctx, opts)
	if err != nil {
		return err
	}

	completed, err := config.Complete()
	if err != nil {
		return err
	}

	// Initialize and prepare storage to be injected into the server for readiness.
	storage, err := completed.Storage.New()
	if err != nil {
		return err
	}

	// Inject storage into the server to coordinate startup.
	server, err := completed.APIs.New(storage)
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		preparedStorage, err := storage.PrepareRun(ctx)
		if err != nil {
			return err
		}

		return preparedStorage.Run(ctx)
	})

	g.Go(func() error {
		preparedServer, err := server.PrepareRun(ctx)
		if err != nil {
			return err
		}

		return preparedServer.Run(ctx)
	})

	err = g.Wait()
	if err != nil {
		logger.Error(err, "internal error: Device API Server exited with error")
		return err
	}

	logger.Info("Device API Server shut down gracefully")

	return nil
}
