package driver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/container-storage-interface/spec/lib/go/csi"
	tnclient "github.com/terricain/truenas-go-sdk/pkg/truenas"
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
	if err := iscsiCheckCaps(req.GetVolumeCapabilities()); err != nil {
		klog.ErrorS(err, "invalid volume capabilities")
		return nil, err
	}

	// Get iSCSI IQN prefix
	globalConfigResponse, _, err := d.client.IscsiGlobalAPI.GetISCSIGlobalConfiguration(ctx).Execute()
	if err != nil {
		klog.ErrorS(err, "failed to get global iSCSI config")
		return nil, status.Errorf(codes.Internal, "failed to get global iSCSI config: %v", err)
	}
	iqnBase := globalConfigResponse.Basename

	// Get portal get portal ip and port
	portalResponse, _, err := d.client.IscsiPortalAPI.GetISCSIPortal(ctx, d.portalID).Execute()
	if err != nil {
		klog.ErrorS(err, "failed to get portal", "portalID", d.portalID)
		return nil, status.Errorf(codes.Internal, "failed to get portal info: %v", err)
	}
	if len(portalResponse.Listen) == 0 {
		klog.ErrorS(err, "portal has no listen addresses", "portalID", d.portalID)
		return nil, status.Errorf(codes.Internal, "failed to get active iSCSI portal: %v", err)
	}
	if len(portalResponse.Listen) > 1 {
		klog.InfoS("portal has more than 1 listening address, using first one", "portalID", d.portalID)
	}
	portalListenItem := portalResponse.Listen[0]
	portalAddr := fmt.Sprintf("%s:%d", portalListenItem.GetIp(), portalListenItem.GetPort())

	volumeID := ISCSIVolumePrefix + req.GetName()

	// Don't care about size for now.
	// TODO(iscsi) check for space
	// Could iterate though a pool/dataset and check quotas but meh
	//
	size, err := extractStorage(req.GetCapacityRange())
	klog.V(5).InfoS("[Debug] raw size requested in bytes", "size", size)
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "invalid capacity range: %v", err)
	}
	sizeGB := size / (1 * giB)
	klog.V(5).InfoS("[Debug] raw size requested in gibibytes", "size", sizeGB)

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
		klog.ErrorS(err, "failed to look for existing datasets")
		return nil, status.Errorf(codes.Internal, "failed to look for existing datasets: %v", err)
	}

	if datasetExists {
		datasetID = existingDataset.Id
		klog.V(5).Info("[Debug] Dataset exists, skipping")
	} else {
		// Create dataset as a Volume
		klog.V(5).Info("[Debug] Dataset does not exist, creating")

		datasetRequest := d.client.DatasetAPI.CreateDataset(ctx).CreateDatasetParams(tnclient.CreateDatasetParams{
			Name:         datasetName,
			Type:         tnclient.PtrString("VOLUME"),
			Volblocksize: tnclient.PtrString("16K"),
			Volsize:      tnclient.PtrInt64(size),
		})
		datasetResponse, _, err2 := datasetRequest.Execute()
		if err2 != nil {
			klog.ErrorS(err2, "failed to create dataset", "datasetName", datasetName)
			return nil, err2
		}
		datasetID = datasetResponse.Id
	}

	// Cleanup functions
	removeDatasetFunc := func() {
		klog.V(5).Info("[Debug] Cleaning up dataset")
		_, err = d.client.DatasetAPI.DeleteDataset(ctx, datasetID).Execute()
		if err != nil {
			klog.ErrorS(err, "failed to clean up dataset", "datasetName", datasetName)
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
		klog.ErrorS(err, "failed to look for existing iSCSI extents")
		return nil, status.Errorf(codes.Internal, "failed to look for existing iSCSI extents: %v", err)
	}

	if extentExists {
		klog.V(5).Info("[Debug] iSCSI extent exists, skipping")
		extentID = existingExtent.Id
	} else {
		klog.V(5).Info("[Debug] iSCSI extent does not exist, creating")

		extentRequest := d.client.IscsiExtentAPI.CreateISCSIExtent(ctx).CreateISCSIExtentParams(tnclient.CreateISCSIExtentParams{
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
			klog.ErrorS(err2, "failed to create iSCSI extent", "extentName", volumeID)
			return nil, err2
		}
		extentID = extentResponse.Id
	}

	removeExtentFunc := func() {
		klog.V(5).Info("[Debug] Cleaning up iSCSI Extent")
		_, err = d.client.IscsiExtentAPI.DeleteISCSIExtent(ctx, extentID).DeleteISCSIExtentParams(tnclient.DeleteISCSIExtentParams{
			Remove: true,
			Force:  true,
		}).Execute()
		if err != nil {
			klog.ErrorS(err, "failed to cleanup iSCSI Extent", "iSCSIExtentID", extentID)
		}
	}
	cleanupFunc = func() {
		removeDatasetFunc()
		removeExtentFunc()
	}

	// Create iSCSI initiator
	existingInitiator, initiatorExists, err := FindISCSIInitiator(ctx, d.client, func(initiator tnclient.ISCSIInitiator) bool {
		return strings.HasPrefix(initiator.GetComment(), volumeID)
	})
	if err != nil {
		cleanupFunc()
		klog.ErrorS(err, "failed to look for existing iSCSI initiators")
		return nil, status.Errorf(codes.Internal, "failed to look for existing iSCSI initiators: %v", err)
	}

	if initiatorExists {
		klog.V(5).Info("[Debug] iSCSI initiator exists, skipping")
		initatorID = existingInitiator.Id
	} else {
		klog.V(5).Info("[Debug] iSCSI initiator does not exist, creating")

		initiatorRequest := d.client.IscsiInitiatorAPI.CreateISCSIInitiator(ctx).CreateISCSIInitiatorParams(tnclient.CreateISCSIInitiatorParams{
			Comment: tnclient.PtrString(volumeID + ": Kubernetes managed iSCSI initiator"),
		})
		initiatorResponse, _, err2 := initiatorRequest.Execute()
		if err2 != nil {
			cleanupFunc()
			klog.ErrorS(err, "failed to create iSCSI initiator", "initiatorName", volumeID)
			return nil, err2
		}
		initatorID = initiatorResponse.Id
	}

	removeInitiatorFunc := func() {
		klog.V(5).Info("[Debug] Cleaning up iSCSI Initiator")
		_, err = d.client.IscsiInitiatorAPI.DeleteISCSIInitiator(ctx, initatorID).Execute()
		if err != nil {
			klog.ErrorS(err, "failed to cleanup iSCSI Initiator", "iSCSIInitiatorID", initatorID)
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
		klog.ErrorS(err, "failed to look for existing iSCSI targets")
		return nil, status.Errorf(codes.Internal, "failed to look for existing iSCSI targets: %v", err)
	}

	if targetExists {
		klog.V(5).Info("[Debug] iSCSI target exists, skipping")
		targetID = existingTarget.Id
	} else {
		klog.V(5).Info("[Debug] iSCSI target does not exist, creating")

		targetRequest := d.client.IscsiTargetAPI.CreateISCSITarget(ctx).CreateISCSITargetParams(tnclient.CreateISCSITargetParams{
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
			klog.ErrorS(err2, "failed to create iSCSI target", "targetName", volumeID)
			return nil, err2
		}
		targetID = targetResponse.Id
	}

	removeTargetFunc := func() {
		klog.V(5).Info("[Debug] Cleaning up iSCSI Target")
		_, err = d.client.IscsiTargetAPI.DeleteISCSITarget(ctx, targetID).Body(true).Execute()
		if err != nil {
			klog.ErrorS(err, "failed to cleanup iSCSI Target", "iSCSITargetID", targetID)
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
		klog.ErrorS(err, "failed to look for existing iSCSI target extents")
		return nil, status.Errorf(codes.Internal, "failed to look for existing iSCSI target extents: %v", err)
	}

	if targetExtentExists {
		klog.V(5).Info("[Debug] iSCSI target extent exists, skipping")
		// targetExtentID = existingTargetExtent.Id
	} else {
		klog.V(5).Info("[Debug] iSCSI target extent does not exist, creating")

		targetExtentRequest := d.client.IscsiTargetextentAPI.CreateISCSITargetExtent(ctx).CreateISCSITargetExtentParams(tnclient.CreateISCSITargetExtentParams{
			Target: targetID,
			Extent: extentID,
		})
		_, _, err2 := targetExtentRequest.Execute()
		if err2 != nil {
			cleanupFunc()
			klog.ErrorS(err, "failed to create iSCSI target extent", "extentID", extentID)
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

func (d *Driver) iscsiDeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) error {
	volumeID := req.GetVolumeId()
	if !strings.HasPrefix(volumeID, ISCSIVolumePrefix) {
		return status.Errorf(codes.NotFound, "Volume ID %s not found", volumeID)
	}

	// Force delete the target first, else delete dataset will spam "device busy"
	existingTarget, targetExists, err := FindISCSITarget(ctx, d.client, func(target tnclient.ISCSITarget) bool {
		return target.GetName() == volumeID
	})
	if err != nil {
		klog.ErrorS(err, "failed to look for existing iSCSI targets")
		return err
	}
	if targetExists {
		_, err = d.client.IscsiTargetAPI.DeleteISCSITarget(ctx, existingTarget.GetId()).Body(true).Execute()
		if err != nil {
			klog.ErrorS(err, "failed to delete iSCSI Target", "iscsi_target_id", existingTarget.GetId())
			return err
		}
	}

	// Deleting the dataset will remove ?
	datasetName := strings.Join([]string{d.iscsiStoragePath, volumeID}, "/")

	existingDataset, datasetExists, err := FindDataset(ctx, d.client, func(dataset tnclient.Dataset) bool {
		return dataset.GetName() == datasetName
	})
	if err != nil {
		klog.ErrorS(err, "failed to look for existing datasets")
		return err
	}

	if datasetExists {
		_, err = d.client.DatasetAPI.DeleteDataset(ctx, existingDataset.GetId()).Execute()
		if err != nil {
			klog.ErrorS(err, "failed to delete Dataset", "dataset_id", existingDataset.GetId())
			return err
		}
	}

	// Deleting the dataset leaves only the initiator
	existingInitiator, initiatorExists, err := FindISCSIInitiator(ctx, d.client, func(initiator tnclient.ISCSIInitiator) bool {
		return strings.HasPrefix(initiator.GetComment(), volumeID)
	})
	if err != nil {
		klog.ErrorS(err, "failed to look for existing iSCSI initiators")
		return err
	}

	if initiatorExists {
		klog.V(5).InfoS("[Debug] cleaning up iSCSI Initiator", "iSCSIInitiatorID", existingInitiator.Id)
		_, err = d.client.IscsiInitiatorAPI.DeleteISCSIInitiator(ctx, existingInitiator.Id).Execute()
		if err != nil {
			klog.ErrorS(err, "failed to cleanup iSCSI Initiator", "iSCSIInitiatorID", existingInitiator.Id)
			return err
		}
	}

	return nil
}

func (d *Driver) iscsiValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID := req.GetVolumeId()
	if !strings.HasPrefix(volumeID, ISCSIVolumePrefix) {
		return nil, status.Errorf(codes.NotFound, "ValidateVolumeCapabilities Volume ID %s not found", volumeID)
	}

	if err := iscsiCheckCaps(req.GetVolumeCapabilities()); err != nil {
		klog.ErrorS(err, "invalid volume capabilities")
		return nil, err
	}

	datasetName := strings.Join([]string{d.iscsiStoragePath, volumeID}, "/")

	// Validate volume context
	foundContextKeys := 0 //nolint:ifshort
	for k := range req.GetVolumeContext() {
		switch k {
		case ISCSIVolumeContextTargetPortal:
			foundContextKeys++
		case ISCSIVolumeContextIQN:
			foundContextKeys++
		case ISCSIVolumeContextLUN:
			foundContextKeys++
		case ISCSIVolumeContextPortals:
			foundContextKeys++
		}
	}
	if foundContextKeys != 4 {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("%s, %s, %s, %s keys missing from volume context", ISCSIVolumeContextTargetPortal, ISCSIVolumeContextIQN, ISCSIVolumeContextLUN, ISCSIVolumeContextPortals))
	}

	// Look for existing dataset
	_, datasetExists, err := FindDataset(ctx, d.client, func(dataset tnclient.Dataset) bool {
		return dataset.GetName() == datasetName
	})
	if err != nil {
		klog.ErrorS(err, "failed to look for existing datasets")
		return nil, status.Errorf(codes.Internal, "failed to look for existing datasets: %v", err)
	}

	if !datasetExists {
		return nil, status.Errorf(codes.NotFound, "ValidateVolumeCapabilities Volume ID %s not found", volumeID)
	}

	caps := make([]*csi.VolumeCapability, 0)
	for _, currentCap := range ISCSIVolumeCapabilites {
		caps = append(caps, &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: currentCap},
		})
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext:      req.GetVolumeContext(),
			VolumeCapabilities: caps,
		},
	}, nil
}

func (d *Driver) iscsiGetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) { //nolint:unparam
	// TODO(iscsi) refactor this out as is pretty much same as in nfsGetCapacity
	resp, _, err := d.client.DatasetAPI.GetDataset(ctx, d.iscsiStoragePath).Execute()
	if err != nil {
		klog.ErrorS(err, "failed to get dataset", "datasetID", d.iscsiStoragePath)
		return nil, status.Errorf(codes.Internal, "Failed to get iSCSI dataset: %s", err.Error())
	}

	available, err := strconv.ParseInt(resp.Available.GetRawvalue(), 10, 64)
	if err != nil {
		klog.ErrorS(err, "failed parse available disk space to int64", "available", resp.Available)
		return nil, status.Errorf(codes.Internal, "Failed to parse available bytes: %s", err.Error())
	}

	return &csi.GetCapacityResponse{
		AvailableCapacity: available,
		MaximumVolumeSize: nil,
		MinimumVolumeSize: wrapperspb.Int64(minimumVolumeSizeInBytes),
	}, nil
}

func (d *Driver) iscsiListVolumes(ctx context.Context) ([]*csi.ListVolumesResponse_Entry, error) {
	// So, we want all the volumes -> extents -> extent target mappings -> targets

	iscsiStoragePrefix := d.iscsiStoragePath + "/"
	datasets, err := FindAllDatasets(ctx, d.client, func(dataset tnclient.Dataset) bool {
		return dataset.GetType() == "VOLUME" && strings.HasPrefix(dataset.GetName(), iscsiStoragePrefix)
	})
	if err != nil {
		klog.ErrorS(err, "failed to get list of datasets")
		return nil, err
	}

	datasetMap := make(map[string]*tnclient.Dataset)
	for _, dataset := range datasets {
		ds := dataset
		zvolPath := "zvol/" + ds.GetName()
		datasetMap[zvolPath] = &ds
	}

	extents, err := FindAllISCSIExtents(ctx, d.client, func(extent tnclient.ISCSIExtent) bool {
		_, exists := datasetMap[extent.GetPath()]
		return exists
	})
	if err != nil {
		klog.ErrorS(err, "failed to get list of iSCSI extents")
		return nil, err
	}

	extentMap := make(map[int32]*tnclient.Dataset)
	for _, extent := range extents {
		extentMap[extent.GetId()] = datasetMap[extent.GetPath()]
	}

	// Get extent target mapping
	extentMappings, err := FindAllISCSITargetExtents(ctx, d.client, func(targetExtent tnclient.ISCSITargetExtent) bool {
		_, exists := extentMap[targetExtent.Extent]
		return exists
	})
	if err != nil {
		klog.ErrorS(err, "failed to get list of iSCSI extent mappings")
		return nil, err
	}

	extentTargetMap := make(map[int32]*tnclient.Dataset)
	for _, mapping := range extentMappings {
		extentTargetMap[mapping.GetTarget()] = extentMap[mapping.GetExtent()]
	}

	targets, err := FindAllISCSITargets(ctx, d.client, func(target tnclient.ISCSITarget) bool {
		_, exists := extentTargetMap[target.GetId()]
		return exists
	})
	if err != nil {
		klog.ErrorS(err, "failed to get list of iSCSI targets")
		return nil, err
	}

	result := make([]*csi.ListVolumesResponse_Entry, 0)

	for _, target := range targets {
		volumeID := target.GetName()
		dataset := extentTargetMap[target.GetId()]

		volsizeComp := dataset.GetVolsize()
		quota, err := strconv.ParseInt(volsizeComp.GetRawvalue(), 10, 64)
		if err != nil {
			klog.ErrorS(err, "Failed parse volume size to int64", "volumeSizeComposite", volsizeComp)
			return nil, err
		}

		result = append(result, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      volumeID,
				CapacityBytes: quota,
			},
		})
	}

	return result, nil
}

func (d *Driver) iscsiNodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) { //nolint:unparam
	// Validate volume context
	foundContextKeys := 0 //nolint:ifshort
	for k := range req.GetVolumeContext() {
		switch k {
		case ISCSIVolumeContextTargetPortal:
			foundContextKeys++
		case ISCSIVolumeContextIQN:
			foundContextKeys++
		case ISCSIVolumeContextLUN:
			foundContextKeys++
		case ISCSIVolumeContextPortals:
			foundContextKeys++
		}
	}
	if foundContextKeys != 4 {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("%s, %s, %s, %s keys missing from volume context", ISCSIVolumeContextTargetPortal, ISCSIVolumeContextIQN, ISCSIVolumeContextLUN, ISCSIVolumeContextPortals))
	}

	// req.GetVolumeCapability().GetMount().GetMountFlags()
	klog.V(5).InfoS("[Debug] getting ISCSI info from request", "request", req)
	iscsiInfo, err := getISCSIInfo(req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	libConfigPath := d.getISCSILibConfigPath(req.GetVolumeId())
	klog.V(5).InfoS("[Debug] generated lib config path", "configPath", libConfigPath)
	diskMounter := getISCSIDiskMounter(iscsiInfo, req)

	util := &ISCSIUtil{}
	klog.V(5).Info("[Debug] Attaching disk")
	if _, err = util.AttachDisk(*diskMounter, libConfigPath); err != nil {
		klog.ErrorS(err, "failed to attach disk")
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *Driver) iscsiNodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) { //nolint:unparam
	targetPath := req.GetTargetPath()

	libConfigPath := d.getISCSILibConfigPath(req.GetVolumeId())
	klog.V(5).InfoS("[Debug] generated lib config path", "configPath", libConfigPath)
	diskUnmounter := getISCSIDiskUnmounter(req)

	iscsiutil := &ISCSIUtil{}
	klog.V(5).Info("[Debug] Detaching disk")
	if err := iscsiutil.DetachDisk(*diskUnmounter, targetPath, libConfigPath); err != nil {
		klog.ErrorS(err, "failed to un-attach disk")
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}
