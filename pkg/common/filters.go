/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package common

type DeviceFilters struct {
	Regions   []string
	Sites     []string
	Locations []string
	Racks     []int32
	Tenants   []string
	Roles     []string

	Virtualisation bool
	Clusters       []string

	Status []string
}
