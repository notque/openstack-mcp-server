// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package nova provides MCP tools for OpenStack Compute (Nova) operations.
package nova

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/availabilityzones"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/quotasets"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Nova tools to the MCP server.
// When readOnly is true, mutating tools (server actions) are not registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly bool) {
	s.AddTool(listServersTool, listServersHandler(provider))
	s.AddTool(getServerTool, getServerHandler(provider))
	s.AddTool(listFlavorsTool, listFlavorsHandler(provider))
	s.AddTool(listKeypairsTool, listKeypairsHandler(provider))
	s.AddTool(getQuotasTool, getQuotasHandler(provider))
	s.AddTool(listAvailabilityZonesTool, listAvailabilityZonesHandler(provider))
	if !readOnly {
		s.AddTool(serverActionTool, serverActionHandler(provider))
		s.AddTool(createServerTool, createServerHandler(provider))
	}
}

// --- Tool Definitions ---

var listServersTool = mcp.NewTool("nova_list_servers",
	mcp.WithDescription("List compute instances (servers) in the current project. Returns server ID, name, status, addresses, and flavor."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("status", mcp.Description("Filter by server status (ACTIVE, SHUTOFF, ERROR, BUILD, etc.)")),
	mcp.WithString("name", mcp.Description("Filter by server name (regex supported)")),
	mcp.WithString("image", mcp.Description("Filter by image UUID")),
	mcp.WithString("flavor", mcp.Description("Filter by flavor UUID or name")),
	mcp.WithString("ip", mcp.Description("Filter by IPv4 address (regex match)")),
	mcp.WithString("availability_zone", mcp.Description("Filter by availability zone")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of servers to return (default: 100)")),
)

var getServerTool = mcp.NewTool("nova_get_server",
	mcp.WithDescription("Get detailed information about a specific compute instance by ID."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("server_id", mcp.Required(), mcp.Description("The UUID of the server to retrieve")),
)

var listFlavorsTool = mcp.NewTool("nova_list_flavors",
	mcp.WithDescription("List available compute flavors (instance types) with their specs: vCPUs, RAM, disk."),
	mcp.WithReadOnlyHintAnnotation(true),
)

var listKeypairsTool = mcp.NewTool("nova_list_keypairs",
	mcp.WithDescription("List SSH keypairs available in the current project. Returns keypair name, fingerprint, public key, and type."),
	mcp.WithReadOnlyHintAnnotation(true),
)

var serverActionTool = mcp.NewTool("nova_server_action",
	mcp.WithDescription("Perform an action on a compute instance: start, stop, reboot, pause, unpause, suspend, resume."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("server_id", mcp.Required(), mcp.Description("The UUID of the server")),
	mcp.WithString("action", mcp.Required(), mcp.Description("Action to perform: start, stop, reboot, pause, unpause, suspend, resume")),
	mcp.WithString("reboot_type", mcp.Description("Reboot type: SOFT or HARD (default: SOFT). Only used with 'reboot' action.")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

// --- Handlers ---

func listServersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		opts := servers.ListOpts{
			Status:           shared.StringParam(request, "status"),
			Name:             shared.StringParam(request, "name"),
			Image:            shared.StringParam(request, "image"),
			Flavor:           shared.StringParam(request, "flavor"),
			IP:               shared.StringParam(request, "ip"),
			AvailabilityZone: shared.StringParam(request, "availability_zone"),
		}

		var maxResults int
		if limit := shared.NumberParam(request, "limit"); limit > 0 {
			opts.Limit = int(limit)
			maxResults = int(limit)
		} else {
			opts.Limit = 100
			maxResults = 100
		}

		var result []map[string]any
		err = servers.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			srvs, err := servers.ExtractServers(page)
			if err != nil {
				return false, err
			}
			for _, s := range srvs {
				result = append(result, map[string]any{
					"id":        s.ID,
					"name":      s.Name,
					"status":    s.Status,
					"addresses": s.Addresses,
					"flavor":    s.Flavor,
					"created":   s.Created,
					"updated":   s.Updated,
					"host_id":   s.HostID,
				})
			}
			// Stop paginating once we have enough results
			return len(result) < maxResults, nil
		})
		if err != nil {
			return shared.ToolError("failed to list servers: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getServerHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		serverID := shared.StringParam(request, "server_id")
		if serverID == "" {
			return shared.ToolError("server_id is required"), nil
		}
		if errResult := shared.ValidateUUID(serverID, "server_id"); errResult != nil {
			return errResult, nil
		}

		srv, err := servers.Get(ctx, client, serverID).Extract()
		if err != nil {
			return shared.ToolError("failed to get server %s: %v", serverID, err), nil
		}

		// SECURITY: Use allowlist of safe fields. The full Server struct contains
		// AdminPass (the admin password set at provisioning) and potentially other
		// sensitive fields from extensions. Never marshal the entire struct.
		safe := map[string]any{
			"id":                srv.ID,
			"name":              srv.Name,
			"status":            srv.Status,
			"tenant_id":         srv.TenantID,
			"user_id":           srv.UserID,
			"addresses":         srv.Addresses,
			"flavor":            srv.Flavor,
			"image":             srv.Image,
			"key_name":          srv.KeyName,
			"created":           srv.Created,
			"updated":           srv.Updated,
			"host_id":           srv.HostID,
			"availability_zone": srv.AvailabilityZone,
			"metadata":          srv.Metadata,
			"security_groups":   srv.SecurityGroups,
			"attached_volumes":  srv.AttachedVolumes,
			"fault":             srv.Fault,
			"tags":              srv.Tags,
		}

		out, err := json.MarshalIndent(safe, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listFlavorsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		var result []map[string]any
		err = flavors.ListDetail(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			flvs, err := flavors.ExtractFlavors(page)
			if err != nil {
				return false, err
			}
			for _, f := range flvs {
				result = append(result, map[string]any{
					"id":    f.ID,
					"name":  f.Name,
					"vcpus": f.VCPUs,
					"ram":   f.RAM,
					"disk":  f.Disk,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list flavors: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listKeypairsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		var result []map[string]any
		err = keypairs.List(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			kps, err := keypairs.ExtractKeyPairs(page)
			if err != nil {
				return false, err
			}
			for _, kp := range kps {
				result = append(result, map[string]any{
					"name":        kp.Name,
					"fingerprint": kp.Fingerprint,
					"public_key":  kp.PublicKey,
					"type":        kp.Type,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list keypairs: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func serverActionHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		serverID := shared.StringParam(request, "server_id")
		action := shared.StringParam(request, "action")

		if serverID == "" || action == "" {
			return shared.ToolError("server_id and action are required"), nil
		}
		if errResult := shared.ValidateUUID(serverID, "server_id"); errResult != nil {
			return errResult, nil
		}

		// Validate action before confirmation.
		validActions := map[string]bool{
			"start": true, "stop": true, "reboot": true,
			"pause": true, "unpause": true, "suspend": true, "resume": true,
		}
		if !validActions[action] {
			return shared.ToolError("unsupported action: %s (valid: start, stop, reboot, pause, unpause, suspend, resume)", action), nil
		}

		preview := fmt.Sprintf("Will %s server %s", strings.ToUpper(action), serverID)
		if action == "reboot" && shared.StringParam(request, "reboot_type") == "HARD" {
			preview = "Will HARD REBOOT server " + serverID
		}
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		switch action {
		case "start":
			err = servers.Start(ctx, client, serverID).ExtractErr()
		case "stop":
			err = servers.Stop(ctx, client, serverID).ExtractErr()
		case "reboot":
			rebootType := servers.SoftReboot
			if shared.StringParam(request, "reboot_type") == "HARD" {
				rebootType = servers.HardReboot
			}
			err = servers.Reboot(ctx, client, serverID, servers.RebootOpts{Type: rebootType}).ExtractErr()
		case "pause":
			err = servers.Pause(ctx, client, serverID).ExtractErr()
		case "unpause":
			err = servers.Unpause(ctx, client, serverID).ExtractErr()
		case "suspend":
			err = servers.Suspend(ctx, client, serverID).ExtractErr()
		case "resume":
			err = servers.Resume(ctx, client, serverID).ExtractErr()
		}

		if err != nil {
			return shared.ToolError("failed to %s server %s: %v", action, serverID, err), nil
		}

		return shared.ToolResult(fmt.Sprintf("Successfully performed '%s' on server %s", action, serverID)), nil
	}
}

// --- Create Server Tool ---

var createServerTool = mcp.NewTool("nova_create_server",
	mcp.WithDescription("Create a new compute instance. Requires confirmation."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("name", mcp.Required(), mcp.Description("Name for the new server")),
	mcp.WithString("flavor_id", mcp.Required(), mcp.Description("The UUID of the flavor (instance type)")),
	mcp.WithString("image_id", mcp.Required(), mcp.Description("The UUID of the image to boot from")),
	mcp.WithString("network_id", mcp.Description("The UUID of the network to attach to")),
	mcp.WithString("key_name", mcp.Description("Name of the SSH keypair to inject")),
	mcp.WithString("security_groups", mcp.Description("Comma-separated list of security group names")),
	mcp.WithString("availability_zone", mcp.Description("Availability zone to launch in")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

func createServerHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		name := shared.StringParam(request, "name")
		if name == "" {
			return shared.ToolError("name is required"), nil
		}

		flavorID := shared.StringParam(request, "flavor_id")
		if flavorID == "" {
			return shared.ToolError("flavor_id is required"), nil
		}
		if errResult := shared.ValidateUUID(flavorID, "flavor_id"); errResult != nil {
			return errResult, nil
		}

		imageID := shared.StringParam(request, "image_id")
		if imageID == "" {
			return shared.ToolError("image_id is required (boot-from-volume without image is not supported)"), nil
		}
		if errResult := shared.ValidateUUID(imageID, "image_id"); errResult != nil {
			return errResult, nil
		}

		networkID := shared.StringParam(request, "network_id")
		if networkID != "" {
			if errResult := shared.ValidateUUID(networkID, "network_id"); errResult != nil {
				return errResult, nil
			}
		}

		keyName := shared.StringParam(request, "key_name")
		secGroupsStr := shared.StringParam(request, "security_groups")
		az := shared.StringParam(request, "availability_zone")

		// Build preview.
		preview := fmt.Sprintf("Will CREATE server '%s' with flavor %s, image %s, network: %s, AZ: %s, keypair: %s",
			name, flavorID, imageID, networkID, az, keyName)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		// Build create options.
		createOpts := servers.CreateOpts{
			Name:             name,
			FlavorRef:        flavorID,
			ImageRef:         imageID,
			AvailabilityZone: az,
		}

		if networkID != "" {
			createOpts.Networks = []servers.Network{{UUID: networkID}}
		}

		if secGroupsStr != "" {
			parts := strings.Split(secGroupsStr, ",")
			secGroups := make([]string, 0, len(parts))
			for _, sg := range parts {
				trimmed := strings.TrimSpace(sg)
				if trimmed != "" {
					secGroups = append(secGroups, trimmed)
				}
			}
			createOpts.SecurityGroups = secGroups
		}

		// Wrap with keypairs extension if key_name provided.
		var createOptsBuilder servers.CreateOptsBuilder = createOpts
		if keyName != "" {
			createOptsBuilder = keypairs.CreateOptsExt{
				CreateOptsBuilder: createOpts,
				KeyName:           keyName,
			}
		}

		srv, err := servers.Create(ctx, client, createOptsBuilder, nil).Extract()
		if err != nil {
			return shared.ToolError("failed to create server: %v", err), nil
		}

		// SECURITY: Use allowlist of safe fields. Never include AdminPass.
		safe := map[string]any{
			"id":                srv.ID,
			"name":              srv.Name,
			"status":            srv.Status,
			"addresses":         srv.Addresses,
			"flavor":            srv.Flavor,
			"image":             srv.Image,
			"key_name":          srv.KeyName,
			"security_groups":   srv.SecurityGroups,
			"availability_zone": srv.AvailabilityZone,
			"created":           srv.Created,
		}

		out, err := json.MarshalIndent(safe, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Quota and Availability Zone Read Tools ---

var getQuotasTool = mcp.NewTool("nova_get_quotas",
	mcp.WithDescription("Get compute quota details (limits and usage) for a project."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("project_id", mcp.Required(), mcp.Description("The UUID of the project to get quotas for")),
)

func getQuotasHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		projectID := shared.StringParam(request, "project_id")
		if projectID == "" {
			return shared.ToolError("project_id is required"), nil
		}
		if errResult := shared.ValidateUUID(projectID, "project_id"); errResult != nil {
			return errResult, nil
		}

		quotaDetail, err := quotasets.GetDetail(ctx, client, projectID).Extract()
		if err != nil {
			return shared.ToolError("failed to get quotas for project %s: %v", projectID, err), nil
		}

		out, err := json.MarshalIndent(quotaDetail, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var listAvailabilityZonesTool = mcp.NewTool("nova_list_availability_zones",
	mcp.WithDescription("List compute availability zones with their status."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func listAvailabilityZonesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		page, err := availabilityzones.List(client).AllPages(ctx)
		if err != nil {
			return shared.ToolError("failed to list availability zones: %v", err), nil
		}

		zones, err := availabilityzones.ExtractAvailabilityZones(page)
		if err != nil {
			return shared.ToolError("failed to extract availability zones: %v", err), nil
		}

		result := make([]map[string]any, 0, len(zones))
		for _, zone := range zones {
			result = append(result, map[string]any{
				"name":      zone.ZoneName,
				"available": zone.ZoneState.Available,
			})
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
