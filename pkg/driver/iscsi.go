package driver

import (
	"context"
	"fmt"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rs/zerolog/log"
	tnclient "github.com/terrycain/truenas-go-sdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	ISCSIVolumePrefix              = "iscsi-"
	ISCSIVolumeContextTargetPortal = "targetPortal"
	ISCSIVolumeContextIQN          = "iqn"
	ISCSIVolumeContextLUN          = "lun"
	ISCSIVolumeContextPortals      = "portals"
)

var ISCSIVolumeCapabilites = []csi.VolumeCapability_AccessMode_Mode{
	csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
	csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
	csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
}

func iscsiHasCapability(capability csi.VolumeCapability_AccessMode_Mode) bool {
	for _, cs := range ISCSIVolumeCapabilites {
		if cs == capability {
			return true
		}
	}
	return false
}

func iscsiCheckCaps(caps []*csi.VolumeCapability) error {
	violations := sets.NewString()
	for _, currentCap := range caps {
		capMode := currentCap.GetAccessMode().GetMode()
		if !iscsiHasCapability(capMode) {
			violations.Insert(fmt.Sprintf("unsupported access mode %s", capMode.String()))
		}

		accessType := currentCap.GetAccessType()
		switch accessType.(type) {
		case *csi.VolumeCapability_Mount:
		default:
			violations.Insert(fmt.Sprintf("unsupported access type %v", accessType))
		}
	}
	if violations.Len() > 0 {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("volume capabilities cannot be satisified: %s", strings.Join(violations.List(), "; ")))
	}
	return nil
}

func (d *Driver) iscsiCreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// Validate iSCSI capabilities
	if err := iscsiCheckCaps(req.VolumeCapabilities); err != nil {
		log.Error().Err(err).Msg("Invalid volume caps")
		return nil, err
	}

	// Get iSCSI IQN prefix
	globalConfigResponse, _, err := d.client.IscsiGlobalApi.GetISCSIGlobalConfiguration(ctx).Execute()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get global iSCSI config")
		return nil, status.Errorf(codes.Internal, "failed to get global iSCSI config: %v", err)
	}
	iqnBase := globalConfigResponse.Basename

	// Get portal get portal ip and port
	portalResponse, _, err := d.client.IscsiPortalApi.GetISCSIPortal(ctx, d.portalID).Execute()
	if err != nil {
		log.Error().Err(err).Int32("portalID", d.portalID).Msg("Failed to get portal")
		return nil, status.Errorf(codes.Internal, "failed to get portal info: %v", err)
	}
	if len(portalResponse.Listen) == 0 {
		log.Error().Err(err).Int32("portalID", d.portalID).Msg("Portal has no listen addresses")
		return nil, status.Errorf(codes.Internal, "failed to get active iSCSI portal: %v", err)
	}
	if len(portalResponse.Listen) > 1 {
		log.Warn().Int32("portalID", d.portalID).Msg("Portal has more than 1 listening address, using first one")
	}
	portalListenItem := portalResponse.Listen[0]
	portalAddr := fmt.Sprintf("%s:%d", portalListenItem.GetIp(), portalListenItem.GetPort())

	volumeID := ISCSIVolumePrefix + req.Name

	// Don't care about size for now.
	// TODO(iscsi) check for space
	// Could iterate though a pool/dataset and check quotas but meh
	//
	size, err := extractStorage(req.CapacityRange)
	log.Debug().Int64("raw_size_requested_bytes", size).Msg("Raw size requested in bytes")
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "invalid capacity range: %v", err)
	}
	sizeGB := size / (1 * giB)
	log.Debug().Int64("raw_size_gib", sizeGB).Msg("Raw size requested in gigabytes")

	datasetName := strings.Join([]string{d.iscsiStoragePath, volumeID}, "/")

	datasetID := ""
	extentID := int32(-1)
	initatorID := int32(-1)
	targetID := int32(-1)
	// targetExtentID := int32(-1)

	// Create dataset
	existingDataset, datasetExists, err := FindDataset(ctx, d.client, func(dataset tnclient.Dataset) bool {
		return dataset.GetName() == datasetName && dataset.GetType() == "VOLUME"
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to look for existing datasets")
		return nil, status.Errorf(codes.Internal, "failed to look for existing datasets: %v", err)
	}

	if datasetExists {
		datasetID = existingDataset.Id
		log.Debug().Msg("Dataset exists, skipping")
	} else {
		// Create dataset as a Volume
		log.Debug().Msg("Dataset does not exist, creating")

		datasetRequest := d.client.DatasetApi.CreateDataset(ctx).CreateDatasetParams(tnclient.CreateDatasetParams{
			Name:         datasetName,
			Type:         tnclient.PtrString("VOLUME"),
			Volblocksize: tnclient.PtrString("16K"),
			Volsize:      tnclient.PtrInt64(size),
		})
		datasetResponse, _, err2 := datasetRequest.Execute()
		if err2 != nil {
			log.Error().Err(err2).Str("datasetName", datasetName).Msg("Failed to create dataset")
			return nil, err2
		}
		datasetID = datasetResponse.Id
	}

	// Cleanup functions
	removeDatasetFunc := func() {
		log.Debug().Msg("Cleaning up dataset")
		_, err = d.client.DatasetApi.DeleteDataset(ctx, datasetID).Execute()
		if err != nil {
			log.Error().Err(err).Str("datasetName", datasetName).Msg("Failed to cleanup dataset")
		}
	}
	cleanupFunc := func() {
		removeDatasetFunc()
	}

	// Create iSCSI extent
	extentPath := "zvol/" + datasetName
	existingExtent, extentExists, err := FindISCSIExtent(ctx, d.client, func(extent tnclient.ISCSIExtent) bool {
		return extent.GetPath() == extentPath
	})
	if err != nil {
		cleanupFunc()
		log.Error().Err(err).Msg("Failed to look for existing iSCSI extents")
		return nil, status.Errorf(codes.Internal, "failed to look for existing iSCSI extents: %v", err)
	}

	if extentExists {
		log.Debug().Msg("iSCSI extent exists, skipping")
		extentID = existingExtent.Id
	} else {
		log.Debug().Msg("iSCSI extent does not exist, creating")

		extentRequest := d.client.IscsiExtentApi.CreateISCSIExtent(ctx).CreateISCSIExtentParams(tnclient.CreateISCSIExtentParams{
			Name:        volumeID,
			Rpm:         tnclient.PtrString("SSD"),
			Type:        "DISK",
			InsecureTpc: tnclient.PtrBool(true),
			Xen:         tnclient.PtrBool(false),
			Comment:     tnclient.PtrString(volumeID + ": Kubernetes managed iSCSI extent"),
			Blocksize:   tnclient.PtrInt32(512),
			Disk:        *tnclient.NewNullableString(tnclient.PtrString(extentPath)),
		})
		extentResponse, _, err2 := extentRequest.Execute()
		if err2 != nil {
			cleanupFunc()
			log.Error().Err(err2).Str("extentName", volumeID).Msg("Failed to create iSCSI extent")
			return nil, err2
		}
		extentID = extentResponse.Id
	}

	removeExtentFunc := func() {
		log.Debug().Msg("Cleaning up iSCSI Extent")
		_, err = d.client.IscsiExtentApi.DeleteISCSIExtent(ctx, extentID).DeleteISCSIExtentParams(tnclient.DeleteISCSIExtentParams{
			Remove: true,
			Force:  true,
		}).Execute()
		if err != nil {
			log.Error().Err(err).Int32("iSCSIExtentID", extentID).Msg("Failed to cleanup iSCSI Extent")
		}
	}
	cleanupFunc = func() {
		removeDatasetFunc()
		removeExtentFunc()
	}

	// Create iSCSI initiator
	existingInitiator, initiatorExists, err := FindISCSIInitiator(ctx, d.client, func(initiator tnclient.ISCSIInitiator) bool {
		return strings.HasPrefix(initiator.Comment, volumeID)
	})
	if err != nil {
		cleanupFunc()
		log.Error().Err(err).Msg("Failed to look for existing iSCSI initiators")
		return nil, status.Errorf(codes.Internal, "failed to look for existing iSCSI initiators: %v", err)
	}

	if initiatorExists {
		log.Debug().Msg("iSCSI initiator exists, skipping")
		initatorID = existingInitiator.Id
	} else {
		log.Debug().Msg("iSCSI initiator does not exist, creating")

		initiatorRequest := d.client.IscsiInitiatorApi.CreateISCSIInitiator(ctx).CreateISCSIInitiatorParams(tnclient.CreateISCSIInitiatorParams{
			Comment: tnclient.PtrString(volumeID + ": Kubernetes managed iSCSI initiator"),
		})
		initiatorResponse, _, err2 := initiatorRequest.Execute()
		if err2 != nil {
			cleanupFunc()
			log.Error().Err(err2).Str("initiatorName", volumeID).Msg("Failed to create iSCSI initiator")
			return nil, err2
		}
		initatorID = initiatorResponse.Id
	}

	removeInitiatorFunc := func() {
		log.Debug().Msg("Cleaning up iSCSI Initiator")
		_, err = d.client.IscsiInitiatorApi.DeleteISCSIInitiator(ctx, initatorID).Execute()
		if err != nil {
			log.Error().Err(err).Int32("iSCSIInitiatorID", initatorID).Msg("Failed to cleanup iSCSI Initiator")
		}
	}
	cleanupFunc = func() {
		removeDatasetFunc()
		removeExtentFunc()
		removeInitiatorFunc()
	}

	// Create iSCSI target
	existingTarget, targetExists, err := FindISCSITarget(ctx, d.client, func(target tnclient.ISCSITarget) bool {
		return target.GetName() == volumeID
	})
	if err != nil {
		cleanupFunc()
		log.Error().Err(err).Msg("Failed to look for existing iSCSI targets")
		return nil, status.Errorf(codes.Internal, "failed to look for existing iSCSI targets: %v", err)
	}

	if targetExists {
		log.Debug().Msg("iSCSI target exists, skipping")
		targetID = existingTarget.Id
	} else {
		log.Debug().Msg("iSCSI target does not exist, creating")

		targetRequest := d.client.IscsiTargetApi.CreateISCSITarget(ctx).CreateISCSITargetParams(tnclient.CreateISCSITargetParams{
			Name:  volumeID,
			Alias: *tnclient.NewNullableString(tnclient.PtrString(volumeID + ": Kubernetes managed iSCSI initiator")),
			Mode:  tnclient.PtrString("ISCSI"),
			Groups: []tnclient.CreateISCSITargetParamsGroupsInner{
				{
					Portal:     d.portalID,
					Initiator:  tnclient.PtrInt32(initatorID),
					Authmethod: "NONE",
				},
			},
		})
		targetResponse, _, err2 := targetRequest.Execute()
		if err2 != nil {
			cleanupFunc()
			log.Error().Err(err2).Str("targetName", volumeID).Msg("Failed to create iSCSI target")
			return nil, err2
		}
		targetID = targetResponse.Id
	}

	removeTargetFunc := func() {
		log.Debug().Msg("Cleaning up iSCSI Target")
		_, err = d.client.IscsiTargetApi.DeleteISCSITarget(ctx, targetID).Body(true).Execute()
		if err != nil {
			log.Error().Err(err).Int32("iSCSITargetID", targetID).Msg("Failed to cleanup iSCSI Target")
		}
	}
	cleanupFunc = func() {
		removeDatasetFunc()
		removeExtentFunc()
		removeInitiatorFunc()
		removeTargetFunc()
	}

	// Create iSCSI target extent mapping
	_, targetExtentExists, err := FindISCSITargetExtent(ctx, d.client, func(targetExtent tnclient.ISCSITargetExtent) bool {
		return targetExtent.Target == targetID && targetExtent.Extent == extentID
	})
	if err != nil {
		cleanupFunc()
		log.Error().Err(err).Msg("Failed to look for existing iSCSI target extents")
		return nil, status.Errorf(codes.Internal, "failed to look for existing iSCSI target extents: %v", err)
	}

	if targetExtentExists {
		log.Debug().Msg("iSCSI target extent exists, skipping")
		// targetExtentID = existingTargetExtent.Id
	} else {
		log.Debug().Msg("iSCSI target extent does not exist, creating")

		targetExtentRequest := d.client.IscsiTargetextentApi.CreateISCSITargetExtent(ctx).CreateISCSITargetExtentParams(tnclient.CreateISCSITargetExtentParams{
			Target: targetID,
			Extent: extentID,
		})
		_, _, err2 := targetExtentRequest.Execute()
		if err2 != nil {
			cleanupFunc()
			log.Error().Err(err2).Int32("targetID", targetID).Int32("extentID", extentID).Msg("Failed to create iSCSI target extent")
			return nil, err2
		}
		// targetExtentID = targetExtentResponse.Id
	}

	// Should be done by now
	iqn := fmt.Sprintf("%s:%s", iqnBase, volumeID) // iqnBase:targetName

	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: size,
			VolumeContext: map[string]string{
				ISCSIVolumeContextTargetPortal: portalAddr,
				ISCSIVolumeContextIQN:          iqn, // iqn.2005-10.org.freenas.ctl:prometheus
				ISCSIVolumeContextLUN:          "0", // We always set it to 0 in the target extent mapping
				ISCSIVolumeContextPortals:      "[]",
			},
		},
	}

	return resp, nil
}
