package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	iscsiLib "github.com/kubernetes-csi/csi-lib-iscsi/iscsi"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/terrycain/truenas-scale-csi/pkg/driver"
)

func main() {
	var (
		endpoint         = flag.String("endpoint", "", "CSI endpoint")
		truenasURL       = flag.String("url", "", "TrueNAS Scale URL (ends with api/v2.0)")
		nfsStoragePath   = flag.String("nfs-storage-path", "", "NFS StoragePool/Dataset path")
		logLevel         = flag.String("log-level", "info", "Log level (info/warn/fatal/error)")
		version          = flag.Bool("version", false, "Print the version and exit")
		controller       = flag.Bool("controller", false, "Serve controller driver, else it will operate as node driver")
		nodeID           = flag.String("node-id", "", "Node ID")
		csiType          = flag.String("type", "", "Type of CSI driver either NFS or ISCSI")
		iscsiStoragePath = flag.String("iscsi-storage-path", "", "iSCSI StoragePool/Dataset path")
		portalID         = flag.Int("portal", -1, "Portal ID")
	)
	flag.Parse()

	if *version {
		fmt.Printf("%s - %s (%s)\n", driver.Version, driver.Commit, driver.GitTreeState)
		os.Exit(0)
	}

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse log level")
	}
	zerolog.SetGlobalLevel(level)

	var drv *driver.Driver

	if *nodeID == "" {
		log.Fatal().Msg("Node ID must be specified")
	}

	if *csiType != "nfs" && *csiType != "iscsi" {
		log.Fatal().Str("type", *csiType).Msg("type must be either NFS or ISCSI")
	}
	isNFS := *csiType == "nfs"

	if !isNFS {
		if *portalID == -1 {
			log.Fatal().Msg("portal flag must be provided")
		}
		if *iscsiStoragePath == "" {
			log.Fatal().Msg("iSCSI storage path flag must be provided")
		}
	} else {
		if *nfsStoragePath == "" {
			log.Fatal().Msg("iSCSI storage path flag must be provided")
		}
	}

	portalID32 := int32(*portalID)

	if *endpoint == "" {
		if isNFS {
			*endpoint = "unix:///var/run/" + driver.NFSDriverName + "/csi.sock"
		} else {
			*endpoint = "unix:///var/run/" + driver.ISCSIDriverName + "/csi.sock"
		}
		log.Info().Str("endpoint", *endpoint).Msg("Endpoint")
	}

	if *controller {
		accessToken := os.Getenv("TRUENAS_TOKEN")

		log.Debug().Msg("Initiating controller driver")
		if drv, err = driver.NewDriver(*endpoint, *truenasURL, accessToken, *nfsStoragePath, *iscsiStoragePath, portalID32, *controller, *nodeID, isNFS); err != nil {
			log.Fatal().Err(err).Msg("Failed to init CSI driver")
		}
	} else {
		if !isNFS && *logLevel == "debug" {
			iscsiLib.EnableDebugLogging(os.Stdout)
		}

		// Node mode doesnt require qnap access
		log.Debug().Msg("Initiating node driver")
		if drv, err = driver.NewDriver(*endpoint, *truenasURL, "", *nfsStoragePath, *iscsiStoragePath, portalID32, *controller, *nodeID, isNFS); err != nil {
			log.Fatal().Err(err).Msg("Failed to init CSI driver")
		}
	}

	if err = run(drv); err != nil {
		log.Error().Err(err).Msg("Failed to run CSI driver")
	}
}

func run(drv *driver.Driver) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-c
		log.Info().Msgf("Caught signal %s", sig.String())
		cancel()
	}()
	return drv.Run(ctx)
}
