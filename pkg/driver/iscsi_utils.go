package driver

import (
	"context"

	tnclient "github.com/terrycain/truenas-go-sdk"
)

type (
	ISCSIExtentMatcher       func(extent tnclient.ISCSIExtent) bool
	ISCSIInitiatorMatcher    func(initiator tnclient.ISCSIInitiator) bool
	ISCSITargetMatcher       func(target tnclient.ISCSITarget) bool
	ISCSITargetExtentMatcher func(targetExtent tnclient.ISCSITargetExtent) bool
)

func FindISCSIExtent(ctx context.Context, client *tnclient.APIClient, fn ISCSIExtentMatcher) (tnclient.ISCSIExtent, bool, error) {
	extents, _, err := client.IscsiExtentApi.ListISCSIExtent(ctx).Execute()
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

func FindISCSIInitiator(ctx context.Context, client *tnclient.APIClient, fn ISCSIInitiatorMatcher) (tnclient.ISCSIInitiator, bool, error) {
	initiators, _, err := client.IscsiInitiatorApi.ListISCSIInitiator(ctx).Execute()
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
	targets, _, err := client.IscsiTargetApi.ListISCSITarget(ctx).Execute()
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

func FindISCSITargetExtent(ctx context.Context, client *tnclient.APIClient, fn ISCSITargetExtentMatcher) (tnclient.ISCSITargetExtent, bool, error) {
	targetExtents, _, err := client.IscsiTargetextentApi.ListISCSITargetExtent(ctx).Execute()
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
