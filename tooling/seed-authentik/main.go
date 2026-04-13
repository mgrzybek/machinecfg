/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later

seed-authentik creates the machinecfg service accounts and groups in an
Authentik instance, and provisions the corresponding users, permissions and
API tokens in a NetBox instance so that the controllers can authenticate.

Usage:

	AUTHENTIK_ENDPOINT=https://authentik.example.com \
	AUTHENTIK_TOKEN=<admin-token> \
	NETBOX_ENDPOINT=https://netbox.example.com \
	NETBOX_TOKEN=<admin-token> \
	go run ./tooling/seed-authentik/

Output: a JSON object mapping each controller username to its NetBox API token.
*/
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	netbox "github.com/netbox-community/go-netbox/v4"
	authentik "goauthentik.io/api/v3"
)

const (
	netboxUpdaterUser  = "machinecfg-netbox-updater"
	netboxUpdaterGroup = "machinecfg-netbox-updater"
	k8sUpdaterUser     = "machinecfg-kubernetes-updater"
	k8sUpdaterGroup    = "machinecfg-kubernetes-updater"
)

// permissionSpec describes a NetBox ObjectPermission to provision.
type permissionSpec struct {
	name        string
	objectTypes []string
	actions     []string
}

// controllerSpec groups everything needed for a single controller.
type controllerSpec struct {
	username string
	group    string
	perms    []permissionSpec
}

var controllers = []controllerSpec{
	{
		username: netboxUpdaterUser,
		group:    netboxUpdaterGroup,
		perms: []permissionSpec{
			{
				name:        "machinecfg-netbox-updater-device",
				objectTypes: []string{"dcim.device"},
				actions:     []string{"view", "change"},
			},
			{
				name:        "machinecfg-netbox-updater-ipam",
				objectTypes: []string{"ipam.ipaddress", "ipam.fhrpgroup", "ipam.fhrpgroupassignment"},
				actions:     []string{"view", "add", "change", "delete"},
			},
		},
	},
	{
		username: k8sUpdaterUser,
		group:    k8sUpdaterGroup,
		perms: []permissionSpec{
			{
				name:        "machinecfg-kubernetes-updater-read",
				objectTypes: []string{"dcim.device", "ipam.ipaddress", "dcim.interface"},
				actions:     []string{"view"},
			},
		},
	},
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	akEndpoint := os.Getenv("AUTHENTIK_ENDPOINT")
	akToken := os.Getenv("AUTHENTIK_TOKEN")
	nbEndpoint := os.Getenv("NETBOX_ENDPOINT")
	nbToken := os.Getenv("NETBOX_TOKEN")
	caBundle := os.Getenv("CA_BUNDLE")

	for _, v := range []struct{ name, val string }{
		{"AUTHENTIK_ENDPOINT", akEndpoint},
		{"AUTHENTIK_TOKEN", akToken},
		{"NETBOX_ENDPOINT", nbEndpoint},
		{"NETBOX_TOKEN", nbToken},
	} {
		if v.val == "" {
			fmt.Fprintf(os.Stderr, "Error: %s is required\n", v.name)
			os.Exit(1)
		}
	}

	httpClient, err := buildHTTPClient(caBundle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to build HTTP client: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	akClient := newAuthentikClient(akEndpoint, akToken, httpClient)

	nbCfg := netbox.NewConfiguration()
	nbCfg.HTTPClient = httpClient
	nbCfg.Host = func() string {
		u, _ := url.Parse(nbEndpoint)
		return u.Host
	}()
	nbCfg.Scheme = func() string {
		u, _ := url.Parse(nbEndpoint)
		return u.Scheme
	}()
	nbCfg.AddDefaultHeader("Authorization", "Token "+nbToken)
	nbClient := netbox.NewAPIClient(nbCfg)

	tokens := make(map[string]string, len(controllers))

	for _, ctrl := range controllers {
		akUserID, err := ensureAuthentikServiceAccount(ctx, akClient, ctrl.username)
		if err != nil {
			slog.Error("failed to ensure authentik service account", "username", ctrl.username, "error", err)
			os.Exit(1)
		}
		slog.Info("authentik service account ready", "username", ctrl.username, "id", akUserID)

		akGroupUUID, err := ensureAuthentikGroup(ctx, akClient, ctrl.group)
		if err != nil {
			slog.Error("failed to ensure authentik group", "group", ctrl.group, "error", err)
			os.Exit(1)
		}
		slog.Info("authentik group ready", "group", ctrl.group, "uuid", akGroupUUID)

		if err := ensureAuthentikUserInGroup(ctx, akClient, akGroupUUID, akUserID); err != nil {
			slog.Error("failed to add user to group", "username", ctrl.username, "group", ctrl.group, "error", err)
			os.Exit(1)
		}
		slog.Info("authentik user in group", "username", ctrl.username, "group", ctrl.group)

		nbUserID, err := ensureNetboxUser(ctx, nbClient, ctrl.username)
		if err != nil {
			slog.Error("failed to ensure netbox user", "username", ctrl.username, "error", err)
			os.Exit(1)
		}
		slog.Info("netbox user ready", "username", ctrl.username, "id", nbUserID)

		for _, perm := range ctrl.perms {
			if err := ensureNetboxPermission(ctx, nbClient, perm, nbUserID); err != nil {
				slog.Error("failed to ensure netbox permission", "permission", perm.name, "error", err)
				os.Exit(1)
			}
			slog.Info("netbox permission ready", "permission", perm.name)
		}

		token, err := ensureNetboxToken(ctx, nbClient, nbUserID, ctrl.username)
		if err != nil {
			slog.Error("failed to ensure netbox token", "username", ctrl.username, "error", err)
			os.Exit(1)
		}
		slog.Info("netbox token ready", "username", ctrl.username)
		tokens[ctrl.username] = token
	}

	if err := json.NewEncoder(os.Stdout).Encode(tokens); err != nil {
		slog.Error("failed to encode output", "error", err)
		os.Exit(1)
	}

	fmt.Println("Seed complete.")
}

// buildHTTPClient returns an *http.Client configured with the given PEM CA
// bundle file. If caBundle is empty, the system certificate pool is used.
func buildHTTPClient(caBundle string) (*http.Client, error) {
	if caBundle == "" {
		return &http.Client{}, nil
	}
	pem, err := os.ReadFile(caBundle)
	if err != nil {
		return nil, fmt.Errorf("read CA bundle %s: %w", caBundle, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no valid certificates found in %s", caBundle)
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool},
	}
	return &http.Client{Transport: transport}, nil
}

// newAuthentikClient builds an Authentik API client from an endpoint URL,
// an admin token, and an optional custom HTTP client (for private CA support).
func newAuthentikClient(endpoint, token string, httpClient *http.Client) *authentik.APIClient {
	u, err := url.Parse(endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid AUTHENTIK_ENDPOINT: %v\n", err)
		os.Exit(1)
	}
	cfg := authentik.NewConfiguration()
	cfg.Host = u.Host
	cfg.Scheme = u.Scheme
	cfg.HTTPClient = httpClient
	cfg.AddDefaultHeader("Authorization", "Bearer "+token)
	return authentik.NewAPIClient(cfg)
}

// ensureAuthentikServiceAccount creates a service-account user in Authentik if
// it does not already exist, and returns its integer primary key.
func ensureAuthentikServiceAccount(ctx context.Context, client *authentik.APIClient, username string) (int32, error) {
	result, _, err := client.CoreApi.CoreUsersList(ctx).Username(username).Execute()
	if err != nil {
		return 0, fmt.Errorf("list users: %w", err)
	}
	if len(result.Results) > 0 {
		return result.Results[0].Pk, nil
	}
	userType := authentik.USERTYPEENUM_SERVICE_ACCOUNT
	req := authentik.NewUserRequest(username, username)
	req.SetType(userType)
	created, _, err := client.CoreApi.CoreUsersCreate(ctx).UserRequest(*req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create service account %s: %w", username, err)
	}
	return created.Pk, nil
}

// ensureAuthentikGroup creates a group in Authentik if it does not already
// exist, and returns its UUID.
func ensureAuthentikGroup(ctx context.Context, client *authentik.APIClient, name string) (string, error) {
	result, _, err := client.CoreApi.CoreGroupsList(ctx).Name(name).Execute()
	if err != nil {
		return "", fmt.Errorf("list groups: %w", err)
	}
	if len(result.Results) > 0 {
		return result.Results[0].Pk, nil
	}
	req := authentik.NewGroupRequest(name)
	created, _, err := client.CoreApi.CoreGroupsCreate(ctx).GroupRequest(*req).Execute()
	if err != nil {
		return "", fmt.Errorf("create group %s: %w", name, err)
	}
	return created.Pk, nil
}

// ensureAuthentikUserInGroup adds a user to a group if they are not already a
// member. It uses MembersByPk to check membership before calling the add API.
func ensureAuthentikUserInGroup(ctx context.Context, client *authentik.APIClient, groupUUID string, userID int32) error {
	result, _, err := client.CoreApi.CoreGroupsList(ctx).MembersByPk([]int32{userID}).Execute()
	if err != nil {
		return fmt.Errorf("list groups for user %d: %w", userID, err)
	}
	for _, g := range result.Results {
		if g.Pk == groupUUID {
			return nil
		}
	}
	req := authentik.NewUserAccountRequest(userID)
	_, err = client.CoreApi.CoreGroupsAddUserCreate(ctx, groupUUID).UserAccountRequest(*req).Execute()
	if err != nil {
		return fmt.Errorf("add user %d to group %s: %w", userID, groupUUID, err)
	}
	return nil
}

// ensureNetboxUser creates a NetBox user if it does not already exist, and
// returns its ID. A random password is generated since the user authenticates
// exclusively via API token.
func ensureNetboxUser(ctx context.Context, client *netbox.APIClient, username string) (int32, error) {
	result, _, err := client.UsersAPI.UsersUsersList(ctx).Username([]string{username}).Execute()
	if err != nil {
		return 0, fmt.Errorf("list users: %w", err)
	}
	if result.Count > 0 {
		return result.Results[0].Id, nil
	}
	password, err := randomHex(16)
	if err != nil {
		return 0, fmt.Errorf("generate password: %w", err)
	}
	req := netbox.NewUserRequest(username, password)
	created, _, err := client.UsersAPI.UsersUsersCreate(ctx).UserRequest(*req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create user %s: %w", username, err)
	}
	return created.Id, nil
}

// ensureNetboxPermission creates a NetBox ObjectPermission if one with the
// given name does not already exist, and assigns it to the given user.
func ensureNetboxPermission(ctx context.Context, client *netbox.APIClient, spec permissionSpec, userID int32) error {
	result, _, err := client.UsersAPI.UsersPermissionsList(ctx).Name([]string{spec.name}).Execute()
	if err != nil {
		return fmt.Errorf("list permissions: %w", err)
	}
	if result.Count > 0 {
		return nil
	}
	req := netbox.NewObjectPermissionRequest(spec.name, spec.objectTypes, spec.actions)
	req.Users = append(req.Users, userID)
	_, _, err = client.UsersAPI.UsersPermissionsCreate(ctx).ObjectPermissionRequest(*req).Execute()
	if err != nil {
		return fmt.Errorf("create permission %s: %w", spec.name, err)
	}
	return nil
}

// ensureNetboxToken creates a NetBox API token for the given user if none
// exists yet. The key is generated locally and set on the request so it can
// be returned to the caller — the NetBox list endpoint does not expose token
// keys, so existing tokens are skipped with a warning.
func ensureNetboxToken(ctx context.Context, client *netbox.APIClient, userID int32, username string) (string, error) {
	result, _, err := client.UsersAPI.UsersTokensList(ctx).User([]string{username}).Execute()
	if err != nil {
		return "", fmt.Errorf("list tokens: %w", err)
	}
	if result.Count > 0 {
		slog.Warn("existing token found — key not retrievable via API; delete it manually and re-run to rotate", "username", username)
		return "<existing-token-key-not-retrievable>", nil
	}
	key, err := randomHex(20) // 20 bytes → 40-char hex, matching NetBox default
	if err != nil {
		return "", fmt.Errorf("generate token key: %w", err)
	}
	userRef := netbox.Int32AsBookmarkRequestUser(&userID)
	req := netbox.NewTokenRequest(userRef)
	req.SetKey(key)
	_, _, err = client.UsersAPI.UsersTokensCreate(ctx).TokenRequest(*req).Execute()
	if err != nil {
		return "", fmt.Errorf("create token for user %s: %w", username, err)
	}
	return key, nil
}

// randomHex returns a cryptographically random hex string of 2*n characters.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
