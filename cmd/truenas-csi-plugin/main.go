package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	flag "github.com/spf13/pflag"
	"k8s.io/component-base/featuregate"

	"github.com/terricain/truenas-scale-csi/pkg/driver"

	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/logs/json"
	"k8s.io/klog/v2"
)

var featureGate = featuregate.NewFeatureGate()

func main() {
	fs := flag.NewFlagSet("truenas-csi-driver", flag.ExitOnError)

	if err := logsapi.RegisterLogFormat(logsapi.JSONLogFormat, json.Factory{}, logsapi.LoggingBetaOptions); err != nil {
		klog.ErrorS(err, "failed to register JSON log format")
	}

	var (
		endpoint         = fs.String("endpoint", "", "CSI endpoint")
		truenasURL       = fs.String("url", "", "TrueNAS Scale URL (ends with api/v2.0)")
		nfsStoragePath   = fs.String("nfs-storage-path", "", "NFS StoragePool/Dataset path")
		version          = fs.Bool("version", false, "Print the version and exit")
		controller       = fs.Bool("controller", false, "Serve controller driver, else it will operate as node driver")
		nodeID           = fs.String("node-id", "", "Node ID")
		csiType          = fs.String("type", "", "Type of CSI driver either NFS or ISCSI")
		iscsiStoragePath = fs.String("iscsi-storage-path", "", "iSCSI StoragePool/Dataset path")
		portalID         = fs.Int("portal", -1, "Portal ID")
		ignoreTLS        = fs.Bool("ignore-tls", false, "Ignore TLS errors")
		driverName       = fs.String("driver-name", "", "CSI Driver name")
	)
	loggingConfig := logsapi.NewLoggingConfiguration()

	if err := logsapi.AddFeatureGates(featureGate); err != nil {
		klog.ErrorS(err, "failed to add feature gates")
	}

	logsapi.AddFlags(loggingConfig, fs)

	if err := fs.Parse(os.Args[1:]); err != nil {
		panic(err)
	}

	if err := logsapi.ValidateAndApply(loggingConfig, featureGate); err != nil {
		klog.ErrorS(err, "failed to validate and apply logging configuration")
	}

	if *version {
		versionInfo, err := driver.GetVersionJSON()
		if err != nil {
			klog.ErrorS(err, "failed to get version")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
		fmt.Println(versionInfo)
		os.Exit(0)
	}
	if *nodeID == "" {
		klog.Error("--node-id must be specified")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	if *csiType != "nfs" && *csiType != "iscsi" {
		klog.Error("--type must be either NFS or ISCSI")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
	isNFS := *csiType == "nfs"

	if !isNFS {
		if *portalID == -1 {
			klog.Error("--portal must be specified")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
		if *iscsiStoragePath == "" {
			klog.Error("--iscsi-storage-path must be specified")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
	} else { //nolint:gocritic
		if *nfsStoragePath == "" {
			klog.Error("--nfs-storage-path must be specified")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
	}

	portalID32 := int32(*portalID)

	if *endpoint == "" {
		klog.Error("--endpoint must be specified")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	if *driverName == "" {
		klog.Error("--driver-name must be specified")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	var drv *driver.Driver
	var err error
	enableDebugLogging := loggingConfig.Verbosity > 4

	if *controller {
		accessToken := os.Getenv("TRUENAS_TOKEN")

		klog.V(5).Info("initiating controller driver")
		if drv, err = driver.NewDriver(*endpoint, *truenasURL, accessToken, *nfsStoragePath, *iscsiStoragePath, portalID32, *controller, *nodeID, isNFS, enableDebugLogging, *ignoreTLS, *driverName); err != nil {
			klog.ErrorS(err, "failed to init CSI driver")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
	} else {
		// Node mode doesnt require qnap access
		klog.V(5).Info("initiating node driver")
		if drv, err = driver.NewDriver(*endpoint, *truenasURL, "", *nfsStoragePath, *iscsiStoragePath, portalID32, *controller, *nodeID, isNFS, enableDebugLogging, *ignoreTLS, *driverName); err != nil {
			klog.ErrorS(err, "failed to init CSI driver")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
	}

	if err = run(drv); err != nil {
		klog.ErrorS(err, "failed to run CSI driver")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
}

func run(drv *driver.Driver) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-c
		klog.Infof("Caught signal %s", sig.String())
		cancel()
	}()
	return drv.Run(ctx)
}
