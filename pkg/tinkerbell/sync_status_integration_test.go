//go:build integration

/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/

package tinkerbell_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tinkerbellKubeObjects "github.com/tinkerbell/tink/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"machinecfg/pkg/tinkerbell"
)

const integrationNamespace = "testing"

// newIntegrationK8sClient builds a real controller-runtime client from the
// default kubeconfig (KUBECONFIG env or ~/.kube/config). Both Tinkerbell CRD
// types and core Kubernetes types are registered so that the test can create
// the testing namespace as well as Hardware objects.
func newIntegrationK8sClient(t *testing.T) client.Client {
	t.Helper()

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		require.NoError(t, err)
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err, "failed to build kubeconfig from %s", kubeconfig)

	s := runtime.NewScheme()
	require.NoError(t, tinkerbellKubeObjects.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	c, err := client.New(cfg, client.Options{Scheme: s})
	require.NoError(t, err, "failed to create K8s client")
	return c
}

// ensureIntegrationNamespace creates the testing namespace if it does not
// already exist. It is idempotent.
func ensureIntegrationNamespace(ctx context.Context, t *testing.T, k8s client.Client) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: integrationNamespace}}
	err := k8s.Create(ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err, "failed to create namespace %s", integrationNamespace)
	}
}

// netboxIDs groups the required NetBox IDs for device creation.
type netboxIDs struct {
	deviceTypeID int32
	siteID       int32
	roleID       int32
	tenantID     int32
}

// discoverNetboxIDs fetches the first available device type, site, role, and
// the "testing" tenant from the real NetBox instance.
func discoverNetboxIDs(ctx context.Context, t *testing.T, nb *netbox.APIClient) netboxIDs {
	t.Helper()

	// Find the "testing" tenant by slug.
	tenantResp, _, err := nb.TenancyAPI.TenancyTenantsList(ctx).Slug([]string{"testing"}).Execute()
	require.NoError(t, err, "failed to list tenants")
	require.NotEmpty(t, tenantResp.Results, "testing tenant not found in NetBox — create it first")

	// Grab the first available device type.
	dtResp, _, err := nb.DcimAPI.DcimDeviceTypesList(ctx).Limit(1).Execute()
	require.NoError(t, err, "failed to list device types")
	require.NotEmpty(t, dtResp.Results, "no device types found in NetBox")

	// Grab the first available site.
	siteResp, _, err := nb.DcimAPI.DcimSitesList(ctx).Limit(1).Execute()
	require.NoError(t, err, "failed to list sites")
	require.NotEmpty(t, siteResp.Results, "no sites found in NetBox")

	// Grab the first available device role.
	roleResp, _, err := nb.DcimAPI.DcimDeviceRolesList(ctx).Limit(1).Execute()
	require.NoError(t, err, "failed to list device roles")
	require.NotEmpty(t, roleResp.Results, "no device roles found in NetBox")

	return netboxIDs{
		deviceTypeID: dtResp.Results[0].Id,
		siteID:       siteResp.Results[0].Id,
		roleID:       roleResp.Results[0].Id,
		tenantID:     tenantResp.Results[0].Id,
	}
}

// createIntegrationNetboxDevice creates a minimal device under the testing
// tenant and registers a cleanup function to delete it at the end of the test.
// It returns the created device ID.
func createIntegrationNetboxDevice(ctx context.Context, t *testing.T, nb *netbox.APIClient, ids netboxIDs, name string, status netbox.DeviceStatusValue) int32 {
	t.Helper()

	deviceType := netbox.Int32AsDeviceBayTemplateRequestDeviceType(&ids.deviceTypeID)
	role := netbox.Int32AsDeviceWithConfigContextRequestRole(&ids.roleID)
	site := netbox.Int32AsDeviceWithConfigContextRequestSite(&ids.siteID)
	tenant := netbox.Int32AsASNRangeRequestTenant(&ids.tenantID)

	req := netbox.NewWritableDeviceWithConfigContextRequest(deviceType, role, site)
	req.SetName(name)
	req.SetStatus(status)
	req.SetTenant(tenant)

	created, _, err := nb.DcimAPI.DcimDevicesCreate(ctx).
		WritableDeviceWithConfigContextRequest(*req).
		Execute()
	require.NoError(t, err, "failed to create test NetBox device %q", name)

	t.Cleanup(func() {
		_, _ = nb.DcimAPI.DcimDevicesDestroy(context.Background(), created.Id).Execute()
	})

	return created.Id
}

// createIntegrationHardware creates a Hardware object in the testing namespace
// and registers a cleanup function to delete it.
func createIntegrationHardware(ctx context.Context, t *testing.T, k8s client.Client, hwName string, deviceID int32, provisionedAnnot *string) {
	t.Helper()

	hw := makeHardware(hwName, integrationNamespace, fmt.Sprintf("%d", deviceID), provisionedAnnot)
	err := k8s.Create(ctx, hw)
	require.NoError(t, err, "failed to create Hardware %s/%s", integrationNamespace, hwName)

	t.Cleanup(func() {
		// Use a fresh object to avoid stale resource version issues.
		del := &tinkerbellKubeObjects.Hardware{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hwName,
				Namespace: integrationNamespace,
			},
		}
		_ = k8s.Delete(context.Background(), del)
	})
}

// skipIfMissingCreds skips the test when NetBox env vars are absent.
func skipIfMissingCreds(t *testing.T) (endpoint, token string) {
	t.Helper()
	token = os.Getenv("NETBOX_TOKEN")
	endpoint = os.Getenv("NETBOX_ENDPOINT")
	if token == "" || endpoint == "" {
		t.Skip("NETBOX_TOKEN and NETBOX_ENDPOINT are required for integration tests")
	}
	return endpoint, token
}

// TestSyncStatusIntegration mirrors the unit-test table but runs against the
// real NetBox instance.  Each sub-test creates its own device and Hardware
// object, which are both cleaned up after the sub-test.
//
// The regression case (provisioned=true + planned status) is explicitly called
// out first.
func TestSyncStatusIntegration(t *testing.T) {
	endpoint, token := skipIfMissingCreds(t)

	ctx := context.Background()
	nb := netbox.NewAPIClientFor(endpoint, token)
	k8s := newIntegrationK8sClient(t)
	ensureIntegrationNamespace(ctx, t, k8s)
	ids := discoverNetboxIDs(ctx, t, nb)

	tests := []struct {
		name                string
		provisionedAnnot    *string
		initialNetboxStatus netbox.DeviceStatusValue
		wantUpdated         bool
		wantFinalStatus     netbox.DeviceStatusValue
	}{
		{
			// THE REGRESSION CASE: device was "planned", not "staged".
			// Before the fix setDeviceStatus checked expectedCurrent==staged and
			// returned updated=false without issuing a PATCH.
			name:                "provisioned=true, device planned → active",
			provisionedAnnot:    ptr("true"),
			initialNetboxStatus: netbox.DEVICESTATUSVALUE_PLANNED,
			wantUpdated:         true,
			wantFinalStatus:     netbox.DEVICESTATUSVALUE_ACTIVE,
		},
		{
			name:                "provisioned=true, device staged → active",
			provisionedAnnot:    ptr("true"),
			initialNetboxStatus: netbox.DEVICESTATUSVALUE_STAGED,
			wantUpdated:         true,
			wantFinalStatus:     netbox.DEVICESTATUSVALUE_ACTIVE,
		},
		{
			name:                "provisioned=true, device already active → no change",
			provisionedAnnot:    ptr("true"),
			initialNetboxStatus: netbox.DEVICESTATUSVALUE_ACTIVE,
			wantUpdated:         false,
			wantFinalStatus:     netbox.DEVICESTATUSVALUE_ACTIVE,
		},
		{
			name:                "annotation absent, device active → staged",
			provisionedAnnot:    nil,
			initialNetboxStatus: netbox.DEVICESTATUSVALUE_ACTIVE,
			wantUpdated:         true,
			wantFinalStatus:     netbox.DEVICESTATUSVALUE_STAGED,
		},
		{
			name:                "annotation absent, device already staged → no change",
			provisionedAnnot:    nil,
			initialNetboxStatus: netbox.DEVICESTATUSVALUE_STAGED,
			wantUpdated:         false,
			wantFinalStatus:     netbox.DEVICESTATUSVALUE_STAGED,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Give each test a unique device and Hardware name to avoid conflicts
			// when sub-tests run in the same namespace.
			uniqueName := fmt.Sprintf("integration-syncstatus-%d", i)

			deviceID := createIntegrationNetboxDevice(ctx, t, nb, ids, uniqueName, tc.initialNetboxStatus)
			createIntegrationHardware(ctx, t, k8s, uniqueName, deviceID, tc.provisionedAnnot)

			results, err := tinkerbell.SyncStatus(k8s, ctx, integrationNamespace, nb)
			require.NoError(t, err)

			// Find the result entry for this specific device (other tests may
			// have left Hardware objects in the namespace if they ran in parallel).
			var result *tinkerbell.SyncStatusResult
			for j := range results {
				if results[j].Hostname == uniqueName {
					result = &results[j]
					break
				}
			}
			require.NotNil(t, result, "no result entry for hostname %q", uniqueName)

			assert.Empty(t, result.Error)
			assert.Equal(t, tc.wantUpdated, result.Updated)

			// Verify the actual NetBox device status.
			device, _, err := nb.DcimAPI.DcimDevicesRetrieve(ctx, deviceID).Execute()
			require.NoError(t, err)
			finalStatus := device.GetStatus()
			assert.Equal(t, tc.wantFinalStatus, finalStatus.GetValue(),
				"unexpected final NetBox status for %q", uniqueName)
		})
	}
}

// TestSyncStatusIntegration_MissingLabel verifies that a Hardware without the
// netbox-device-id label produces an error entry and leaves NetBox unchanged.
func TestSyncStatusIntegration_MissingLabel(t *testing.T) {
	endpoint, token := skipIfMissingCreds(t)

	ctx := context.Background()
	nb := netbox.NewAPIClientFor(endpoint, token)
	k8s := newIntegrationK8sClient(t)
	ensureIntegrationNamespace(ctx, t, k8s)

	hwName := "integration-missing-label"
	hw := &tinkerbellKubeObjects.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hwName,
			Namespace: integrationNamespace,
			Annotations: map[string]string{
				"v1alpha1.tinkerbell.org/provisioned": "true",
			},
			// No netbox-device-id label.
		},
	}
	err := k8s.Create(ctx, hw)
	require.NoError(t, err)
	t.Cleanup(func() {
		del := &tinkerbellKubeObjects.Hardware{ObjectMeta: metav1.ObjectMeta{Name: hwName, Namespace: integrationNamespace}}
		_ = k8s.Delete(context.Background(), del)
	})

	results, err := tinkerbell.SyncStatus(k8s, ctx, integrationNamespace, nb)
	require.NoError(t, err)

	var result *tinkerbell.SyncStatusResult
	for i := range results {
		if results[i].Hostname == hwName {
			result = &results[i]
			break
		}
	}
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "netbox-device-id")
	assert.False(t, result.Updated)
}
