package driver

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/container-storage-interface/spec/lib/go/csi"
	tnclient "github.com/terrycain/truenas-go-sdk/pkg/truenas"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"
	mountutils "k8s.io/mount-utils"
)

const (
	NFSVolumePrefix                 = "nfs-"
	NFSVolumeContextParamMountPoint = "mountPoint"
	NFSVolumeContextParamHost       = "host"
)

var NFSVolumeCapabilites = []csi.VolumeCapability_AccessMode_Mode{
	csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
	csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
	csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
	csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
	csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
}

func nfsHasCapability(capability csi.VolumeCapability_AccessMode_Mode) bool {
	for _, cs := range NFSVolumeCapabilites {
		if cs == capability {
			return true
		}
	}
	return false
}

func nfsCheckCaps(caps []*csi.VolumeCapability) error {
	violations := sets.NewString()
	for _, currentCap := range caps {
		capMode := currentCap.GetAccessMode().GetMode()
		if !nfsHasCapability(capMode) {
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

func (d *Driver) nfsCreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// Validate NFS capabilities
	if err := nfsCheckCaps(req.GetVolumeCapabilities()); err != nil {
		klog.ErrorS(err, "invalid volume capabilities")
		return nil, err
	}

	volumeID := NFSVolumePrefix + req.GetName()

	// Don't care about size for now.
	// Could iterate though a pool/dataset and check quotas but meh
	//
	size, err := extractStorage(req.GetCapacityRange())
	klog.V(5).InfoS("[Debug] Raw size requested in bytes", "rawSizeRequestedBytes", size)
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "invalid capacity range: %v", err)
	}
	sizeGB := size / (1 * giB)
	klog.V(5).InfoS("[Debug] Raw size requested in gigabytes", "rawSizeGibibytes", sizeGB)

	datasetName := strings.Join([]string{d.nfsStoragePath, volumeID}, "/")
	datasetMountpoint := ""

	// Look for existing dataset
	existingDataset, datasetExists, err := FindDataset(ctx, d.client, func(dataset tnclient.Dataset) bool {
		return dataset.GetName() == datasetName
	})
	if err != nil {
		klog.ErrorS(err, "failed to look for existing datasets")
		return nil, status.Errorf(codes.Internal, "failed to look for existing datasets: %v", err)
	}

	if datasetExists {
		datasetMountpoint = existingDataset.GetMountpoint()
		klog.V(5).Info("[Debug] Dataset exists, skipping")
	} else {
		klog.V(5).Info("[Debug] Dataset does not exist, creating")

		datasetRequest := d.client.DatasetAPI.CreateDataset(ctx).CreateDatasetParams(tnclient.CreateDatasetParams{
			Name:              datasetName,
			Casesensitivity:   tnclient.PtrString("SENSITIVE"),
			Copies:            tnclient.PtrInt32(1),
			InheritEncryption: tnclient.PtrBool(true),
			ShareType:         tnclient.PtrString("GENERIC"),
			Refquota:          tnclient.PtrInt64(size),
		})
		datasetResponse, _, err2 := datasetRequest.Execute()
		if err2 != nil {
			klog.ErrorS(err2, "failed to create dataset", "datasetName", datasetName)
			return nil, err2
		}
		datasetMountpoint = datasetResponse.GetMountpoint()
	}

	_, shareExists, err := FindNFSShare(ctx, d.client, func(share tnclient.ShareNFS) bool {
		return NormaliseNFSShareMountpaths(share) == datasetMountpoint
	})
	if err != nil {
		klog.ErrorS(err, "failed to look for existing NFS shares")
		return nil, status.Errorf(codes.Internal, "failed to look for existing NFS shares: %v", err)
	}

	if !shareExists {
		sharingRequest := d.client.SharingAPI.CreateShareNFS(ctx).CreateShareNFSParams(tnclient.CreateShareNFSParams{
			Paths:        []string{datasetMountpoint},
			Path:         tnclient.PtrString(datasetMountpoint),
			Comment:      tnclient.PtrString(fmt.Sprintf("Share for Kubernetes PV %s", req.GetName())),
			Enabled:      tnclient.PtrBool(true),
			Ro:           tnclient.PtrBool(false),
			MaprootGroup: tnclient.PtrString("root"),
			MaprootUser:  tnclient.PtrString("root"),
			// Can't use additionalProperties
		})
		_, _, err = sharingRequest.Execute()
		if err != nil {
			klog.ErrorS(err, "failed to create NFS share", "datasetName", datasetName, "mountpoint", datasetMountpoint)
			return nil, err
		}
	}

	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: size,
			VolumeContext: map[string]string{
				NFSVolumeContextParamMountPoint: datasetMountpoint,
				NFSVolumeContextParamHost:       d.address,
				// "options": "",
			},
		},
	}

	return resp, nil
}

func (d *Driver) nfsDeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) error {
	volumeID := req.GetVolumeId()
	if !strings.HasPrefix(volumeID, NFSVolumePrefix) {
		return status.Errorf(codes.NotFound, "Volume ID %s not found", volumeID)
	}

	// Deleting the dataset will remove the NFS share :)
	datasetName := strings.Join([]string{d.nfsStoragePath, volumeID}, "/")

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
			klog.ErrorS(err, "failed to delete Dataset", "datasetID", existingDataset.GetId())
			return err
		}
	}

	return nil
}

func (d *Driver) nfsValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID := req.GetVolumeId()
	if !strings.HasPrefix(volumeID, NFSVolumePrefix) {
		return nil, status.Errorf(codes.NotFound, "ValidateVolumeCapabilities Volume ID %s not found", volumeID)
	}

	if err := nfsCheckCaps(req.GetVolumeCapabilities()); err != nil {
		klog.ErrorS(err, "invalid volume caps")
		return nil, err
	}

	datasetName := strings.Join([]string{d.nfsStoragePath, volumeID}, "/")

	// Validate volume context
	foundContextKeys := 0 //nolint:ifshort
	for k := range req.GetVolumeContext() {
		switch k {
		case NFSVolumeContextParamHost:
			foundContextKeys++
		case NFSVolumeContextParamMountPoint:
			foundContextKeys++
		}
	}
	if foundContextKeys != 2 {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("%s, %s keys missing from volume context", NFSVolumeContextParamHost, NFSVolumeContextParamMountPoint))
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
	for _, currentCap := range NFSVolumeCapabilites {
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

func (d *Driver) nfsGetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) { //nolint:unparam
	resp, _, err := d.client.DatasetAPI.GetDataset(ctx, d.nfsStoragePath).Execute()
	if err != nil {
		klog.ErrorS(err, "failed to get dataset", "datasetID", d.nfsStoragePath)
		return nil, status.Errorf(codes.Internal, "Failed to get NFS dataset: %s", err.Error())
	}

	available, err := strconv.ParseInt(resp.Available.GetRawvalue(), 10, 64)
	if err != nil {
		klog.ErrorS(err, "failed parse available to int64", "available", resp.Available)
		return nil, status.Errorf(codes.Internal, "Failed to parse available bytes: %s", err.Error())
	}

	return &csi.GetCapacityResponse{
		AvailableCapacity: available,
		MaximumVolumeSize: nil,
		MinimumVolumeSize: wrapperspb.Int64(minimumVolumeSizeInBytes),
	}, nil
}

func (d *Driver) nfsListVolumes(ctx context.Context) ([]*csi.ListVolumesResponse_Entry, error) {
	// Get all mountpoints that are part of Kube datasets
	nfsStoragePrefix := d.nfsStoragePath + "/"
	datasets, err := FindAllDatasets(ctx, d.client, func(dataset tnclient.Dataset) bool {
		return strings.HasPrefix(dataset.GetName(), nfsStoragePrefix)
	})
	if err != nil {
		klog.ErrorS(err, "failed to get list of datasets")
		return nil, err
	}

	mountpointDataset := make(map[string]tnclient.Dataset)
	for _, dataset := range datasets {
		ds := dataset
		mountpointDataset[dataset.GetMountpoint()] = ds
	}

	shares, err := FindAllNFSShares(ctx, d.client, func(share tnclient.ShareNFS) bool {
		path := NormaliseNFSShareMountpaths(share)
		if len(path) == 0 {
			return false
		}
		_, exists := mountpointDataset[path]

		return exists
	})
	if err != nil {
		klog.ErrorS(err, "failed to get list of NFS shares")
		return nil, err
	}

	result := make([]*csi.ListVolumesResponse_Entry, 0)

	for _, share := range shares {
		path := NormaliseNFSShareMountpaths(share)
		dataset := mountpointDataset[path] // we know this exists by this point
		volumeID := strings.TrimPrefix(dataset.GetName(), nfsStoragePrefix)

		quotaComp := dataset.GetRefquota()
		quota, err := strconv.ParseInt(quotaComp.GetRawvalue(), 10, 64)
		if err != nil {
			klog.ErrorS(err, "failed parse quota to int64", "datasetQuotaComposite", quotaComp)
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

func (d *Driver) nfsNodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) { //nolint:unparam
	volCap := req.GetVolumeCapability()
	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	mountOptions := volCap.GetMount().GetMountFlags()
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	var server, baseDir string
	for k, v := range req.GetVolumeContext() {
		switch k {
		case NFSVolumeContextParamHost:
			server = v
		case NFSVolumeContextParamMountPoint:
			baseDir = v
		}
	}

	if server == "" {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("%v is a required parameter", NFSVolumeContextParamHost))
	}
	if baseDir == "" {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("%v is a required parameter", NFSVolumeContextParamMountPoint))
	}

	server = getServerFromSource(server)
	source := fmt.Sprintf("%s:%s", server, baseDir)

	notMnt, err := d.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(targetPath, os.FileMode(0o777)); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	klog.V(5).InfoS("[Debug] mounting options", "volumeID", volumeID, "nfsSource", source, "targetPath", targetPath, "mountOptions", mountOptions)
	err = d.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.InfoS("mounting NFS volume success", "volumeID", volumeID, "nfsSource", source, "targetPath", targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *Driver) nfsNodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) { //nolint:unparam
	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()
	klog.V(5).InfoS("[Debug] unmounting volume", "volumeID", volumeID, "targetPath", targetPath)
	err := mountutils.CleanupMountPoint(targetPath, d.mounter, true)
	if err != nil {
		klog.ErrorS(err, "failed unmounting volume", "volumeID", volumeID, "targetPath", targetPath)
		return nil, status.Errorf(codes.Internal, "failed to unmount target %q: %v", targetPath, err)
	}
	klog.InfoS("unmounting NFS volume success", "volumeID", volumeID)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}
