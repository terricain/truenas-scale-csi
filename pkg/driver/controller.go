package driver

import (
	"context"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	_   = iota
	kiB = 1 << (10 * iota)
	miB
	giB
	tiB
)

const (
	// MinimumVolumeSizeInBytes is used to validate that the user is not trying
	// to create a volume that is smaller than what we support.
	minimumVolumeSizeInBytes int64 = 1 * giB

	// MaximumVolumeSizeInBytes is used to validate that the user is not trying
	// to create a volume that is larger than what we support.
	maximumVolumeSizeInBytes int64 = 128 * giB

	// DefaultVolumeSizeInBytes is used when the user did not provide a size or
	// the size they provided did not satisfy our requirements.
	defaultVolumeSizeInBytes int64 = 16 * giB
)

func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name must be provided")
	}

	if req.VolumeCapabilities == nil || len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume capabilities must be provided")
	}

	if d.isNFS {
		return d.nfsCreateVolume(ctx, req)
	}
	return d.iscsiCreateVolume(ctx, req)
}

func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}

	switch {
	case strings.HasPrefix(req.VolumeId, NFSVolumePrefix):
		if err := d.nfsDeleteVolume(ctx, req); err != nil {
			return nil, status.Errorf(codes.Internal, "Caught error while deleting volume: %s. %s", req.VolumeId, err.Error())
		}
	case strings.HasPrefix(req.VolumeId, ISCSIVolumePrefix):
		if err := d.iscsiDeleteVolume(ctx, req); err != nil {
			return nil, status.Errorf(codes.Internal, "Caught error while deleting volume: %s. %s", req.VolumeId, err.Error())
		}
	default:
		return nil, status.Errorf(codes.Unknown, "Unknown volume type: %s", req.VolumeId)
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if d.isNFS {
		return d.nfsValidateVolumeCapabilities(ctx, req)
	}
	return d.iscsiValidateVolumeCapabilities(ctx, req)
}

func (d *Driver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	var volumes []*csi.ListVolumesResponse_Entry
	var err error

	if d.isNFS {
		volumes, err = d.nfsListVolumes(ctx)
	} else {
		volumes, err = d.iscsiListVolumes(ctx)
	}

	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to list NFS shares")
	}

	// TODO somehow paginate over 2 sets of data
	if req.GetMaxEntries() != 0 {
		return nil, status.Error(codes.Unimplemented, "Pagination isnt implemented")
	}

	return &csi.ListVolumesResponse{
		Entries:   volumes,
		NextToken: "",
	}, nil
}

func (d *Driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	if d.isNFS {
		return d.nfsGetCapacity(ctx, req)
	}
	return d.iscsiGetCapacity(ctx, req)
}

func (d *Driver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	newCap := func(capType csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: capType,
				},
			},
		}
	}

	caps := make([]*csi.ControllerServiceCapability, 0)
	for _, currentCap := range []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		// csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_GET_CAPACITY,
		// csi.ControllerServiceCapability_RPC_GET_VOLUME,
		// csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		// csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES,
		// 	ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT
		//	ControllerServiceCapability_RPC_LIST_SNAPSHOTS
	} {
		caps = append(caps, newCap(currentCap))
	}

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}

	return resp, nil
}

func (d *Driver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	// TODO(nfs,iscsi)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	// TODO(nfs,iscsi)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	// TODO(nfs,iscsi)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	// TODO(nfs,iscsi)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	// TODO(nfs,iscsi)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented") // Not needed
}

func (d *Driver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented") // Not needed
}
