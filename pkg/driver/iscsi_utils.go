package driver

import (
	"context"

	tnclient "github.com/terricain/truenas-go-sdk/pkg/truenas"
)

type (
	ISCSIExtentMatcher       func(extent tnclient.ISCSIExtent) bool
	ISCSIInitiatorMatcher    func(initiator tnclient.ISCSIInitiator) bool
	ISCSITargetMatcher       func(target tnclient.ISCSITarget) bool
	ISCSITargetExtentMatcher func(targetExtent tnclient.ISCSITargetExtent) bool
)

func FindISCSIExtent(ctx context.Context, client *tnclient.APIClient, fn ISCSIExtentMatcher) (tnclient.ISCSIExtent, bool, error) {
	extents, _, err := client.IscsiExtentAPI.ListISCSIExtent(ctx).Execute()
	if err != nil {
		return tnclient.ISCSIExtent{}, false, err
	}

	for _, extent := range extents {
		if fn(extent) {
			return extent, true, nil
		}
	}

	return tnclient.ISCSIExtent{}, false, nil
}

func FindAllISCSIExtents(ctx context.Context, client *tnclient.APIClient, fn ISCSIExtentMatcher) ([]tnclient.ISCSIExtent, error) {
	extents, _, err := client.IscsiExtentAPI.ListISCSIExtent(ctx).Execute()
	if err != nil {
		return []tnclient.ISCSIExtent{}, err
	}

	result := make([]tnclient.ISCSIExtent, 0)

	for _, extent := range extents {
		if fn(extent) {
			result = append(result, extent)
		}
	}

	return result, nil
}

func FindISCSIInitiator(ctx context.Context, client *tnclient.APIClient, fn ISCSIInitiatorMatcher) (tnclient.ISCSIInitiator, bool, error) {
	initiators, _, err := client.IscsiInitiatorAPI.ListISCSIInitiator(ctx).Execute()
	if err != nil {
		return tnclient.ISCSIInitiator{}, false, err
	}

	for _, initiator := range initiators {
		if fn(initiator) {
			return initiator, true, nil
		}
	}

	return tnclient.ISCSIInitiator{}, false, nil
}

func FindISCSITarget(ctx context.Context, client *tnclient.APIClient, fn ISCSITargetMatcher) (tnclient.ISCSITarget, bool, error) {
	targets, _, err := client.IscsiTargetAPI.ListISCSITarget(ctx).Execute()
	if err != nil {
		return tnclient.ISCSITarget{}, false, err
	}

	for _, target := range targets {
		if fn(target) {
			return target, true, nil
		}
	}

	return tnclient.ISCSITarget{}, false, nil
}

func FindAllISCSITargets(ctx context.Context, client *tnclient.APIClient, fn ISCSITargetMatcher) ([]tnclient.ISCSITarget, error) {
	targets, _, err := client.IscsiTargetAPI.ListISCSITarget(ctx).Execute()
	if err != nil {
		return []tnclient.ISCSITarget{}, err
	}

	result := make([]tnclient.ISCSITarget, 0)

	for _, target := range targets {
		if fn(target) {
			result = append(result, target)
		}
	}

	return result, nil
}

func FindISCSITargetExtent(ctx context.Context, client *tnclient.APIClient, fn ISCSITargetExtentMatcher) (tnclient.ISCSITargetExtent, bool, error) {
	targetExtents, _, err := client.IscsiTargetextentAPI.ListISCSITargetExtent(ctx).Execute()
	if err != nil {
		return tnclient.ISCSITargetExtent{}, false, err
	}

	for _, targetExtent := range targetExtents {
		if fn(targetExtent) {
			return targetExtent, true, nil
		}
	}

	return tnclient.ISCSITargetExtent{}, false, nil
}

func FindAllISCSITargetExtents(ctx context.Context, client *tnclient.APIClient, fn ISCSITargetExtentMatcher) ([]tnclient.ISCSITargetExtent, error) {
	targetextents, _, err := client.IscsiTargetextentAPI.ListISCSITargetExtent(ctx).Execute()
	if err != nil {
		return []tnclient.ISCSITargetExtent{}, err
	}

	result := make([]tnclient.ISCSITargetExtent, 0)

	for _, targetextent := range targetextents {
		if fn(targetextent) {
			result = append(result, targetextent)
		}
	}

	return result, nil
}
