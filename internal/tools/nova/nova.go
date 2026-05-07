// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package nova provides MCP tools for OpenStack Compute (Nova) operations.
package nova

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/aggregates"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/availabilityzones"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/hypervisors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/instanceactions"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/quotasets"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servergroups"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/services"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/volumeattach"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Nova tools to the MCP server.
// When readOnly is true, mutating tools (server actions) are not registered.
// When admin is true, admin-only tools (hypervisors, services, aggregates) are registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly bool, admin bool) {
	s.AddTool(listServersTool, listServersHandler(provider))
	s.AddTool(getServerTool, getServerHandler(provider))
	s.AddTool(listFlavorsTool, listFlavorsHandler(provider))
	s.AddTool(listKeypairsTool, listKeypairsHandler(provider))
	s.AddTool(getQuotasTool, getQuotasHandler(provider))
	s.AddTool(listAvailabilityZonesTool, listAvailabilityZonesHandler(provider))
	s.AddTool(listInstanceActionsTool, listInstanceActionsHandler(provider))
	s.AddTool(listServerGroupsTool, listServerGroupsHandler(provider))
	s.AddTool(listVolumeAttachmentsTool, listVolumeAttachmentsHandler(provider))
	if !readOnly {
		s.AddTool(serverActionTool, serverActionHandler(provider))
		s.AddTool(createServerTool, createServerHandler(provider))
	}
	if admin {
		s.AddTool(listHypervisorsTool, listHypervisorsHandler(provider))
		s.AddTool(getHypervisorTool, getHypervisorHandler(provider))
		s.AddTool(listServicesTool, listServicesHandler(provider))
		s.AddTool(listAggregatesTool, listAggregatesHandler(provider))
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

// --- Instance Actions, Server Groups, Volume Attachments ---

var listInstanceActionsTool = mcp.NewTool("nova_list_instance_actions",
	mcp.WithDescription("List actions performed on a server. Returns request ID, action, instance UUID, start time, user ID, and message."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("server_id", mcp.Required(), mcp.Description("The UUID of the server to list actions for")),
)

func listInstanceActionsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
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

		var result []map[string]any
		err = instanceactions.List(client, serverID, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			actions, err := instanceactions.ExtractInstanceActions(page)
			if err != nil {
				return false, err
			}
			for _, a := range actions {
				result = append(result, map[string]any{
					"request_id":    a.RequestID,
					"action":        a.Action,
					"instance_uuid": a.InstanceUUID,
					"start_time":    a.StartTime,
					"user_id":       a.UserID,
					"message":       a.Message,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list instance actions for server %s: %v", serverID, err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var listServerGroupsTool = mcp.NewTool("nova_list_server_groups",
	mcp.WithDescription("List server groups (anti-affinity, affinity) in the current project. Returns ID, name, policies, members, and metadata."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func listServerGroupsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		var result []map[string]any
		err = servergroups.List(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			groups, err := servergroups.ExtractServerGroups(page)
			if err != nil {
				return false, err
			}
			for _, g := range groups {
				result = append(result, map[string]any{
					"id":       g.ID,
					"name":     g.Name,
					"policies": g.Policies,
					"members":  g.Members,
					"metadata": g.Metadata,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list server groups: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var listVolumeAttachmentsTool = mcp.NewTool("nova_list_volume_attachments",
	mcp.WithDescription("List volumes attached to a server. Returns attachment ID, volume ID, server ID, and device."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("server_id", mcp.Required(), mcp.Description("The UUID of the server to list volume attachments for")),
)

func listVolumeAttachmentsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
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

		var result []map[string]any
		err = volumeattach.List(client, serverID).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			attachments, err := volumeattach.ExtractVolumeAttachments(page)
			if err != nil {
				return false, err
			}
			for _, a := range attachments {
				result = append(result, map[string]any{
					"id":        a.ID,
					"volume_id": a.VolumeID,
					"server_id": a.ServerID,
					"device":    a.Device,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list volume attachments for server %s: %v", serverID, err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Admin Tools: Hypervisors, Services, Aggregates ---

var listHypervisorsTool = mcp.NewTool("nova_list_hypervisors",
	mcp.WithDescription("[Admin] List hypervisors. Requires admin role. Returns ID, hostname, status, state, vCPUs, memory, running VMs, and type."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func listHypervisorsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		var result []map[string]any
		err = hypervisors.List(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			hvs, err := hypervisors.ExtractHypervisors(page)
			if err != nil {
				return false, err
			}
			for _, h := range hvs {
				// SECURITY: Use field allowlist. Exclude HostIP, ServiceHost.
				result = append(result, map[string]any{
					"id":                  h.ID,
					"hypervisor_hostname": h.HypervisorHostname,
					"status":              h.Status,
					"state":               h.State,
					"vcpus":               h.VCPUs,
					"vcpus_used":          h.VCPUsUsed,
					"memory_mb":           h.MemoryMB,
					"memory_mb_used":      h.MemoryMBUsed,
					"running_vms":         h.RunningVMs,
					"hypervisor_type":     h.HypervisorType,
					"hypervisor_version":  h.HypervisorVersion,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list hypervisors: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var getHypervisorTool = mcp.NewTool("nova_get_hypervisor",
	mcp.WithDescription("[Admin] Get hypervisor details. Requires admin role. Returns ID, hostname, status, state, vCPUs, memory, running VMs, and type."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("hypervisor_id", mcp.Required(), mcp.Description("The ID of the hypervisor (UUID or integer ID)")),
)

func getHypervisorHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		hypervisorID := shared.StringParam(request, "hypervisor_id")
		if hypervisorID == "" {
			return shared.ToolError("hypervisor_id is required"), nil
		}

		h, err := hypervisors.Get(ctx, client, hypervisorID).Extract()
		if err != nil {
			return shared.ToolError("failed to get hypervisor %s: %v", hypervisorID, err), nil
		}

		// SECURITY: Use field allowlist. Exclude HostIP, ServiceHost.
		safe := map[string]any{
			"id":                  h.ID,
			"hypervisor_hostname": h.HypervisorHostname,
			"status":              h.Status,
			"state":               h.State,
			"vcpus":               h.VCPUs,
			"vcpus_used":          h.VCPUsUsed,
			"memory_mb":           h.MemoryMB,
			"memory_mb_used":      h.MemoryMBUsed,
			"running_vms":         h.RunningVMs,
			"hypervisor_type":     h.HypervisorType,
			"hypervisor_version":  h.HypervisorVersion,
		}

		out, err := json.MarshalIndent(safe, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var listServicesTool = mcp.NewTool("nova_list_services",
	mcp.WithDescription("[Admin] List compute services. Requires admin role. Returns ID, binary, host, zone, status, state, and updated_at."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("binary", mcp.Description("Filter by service binary (e.g. 'nova-compute', 'nova-scheduler')")),
	mcp.WithString("host", mcp.Description("Filter by host name")),
)

func listServicesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		opts := services.ListOpts{
			Binary: shared.StringParam(request, "binary"),
			Host:   shared.StringParam(request, "host"),
		}

		var result []map[string]any
		err = services.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			svcs, err := services.ExtractServices(page)
			if err != nil {
				return false, err
			}
			for _, svc := range svcs {
				result = append(result, map[string]any{
					"id":         svc.ID,
					"binary":     svc.Binary,
					"host":       svc.Host,
					"zone":       svc.Zone,
					"status":     svc.Status,
					"state":      svc.State,
					"updated_at": svc.UpdatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list compute services: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var listAggregatesTool = mcp.NewTool("nova_list_aggregates",
	mcp.WithDescription("[Admin] List host aggregates. Requires admin role. Returns ID, name, availability zone, hosts, metadata, and timestamps."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func listAggregatesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		page, err := aggregates.List(client).AllPages(ctx)
		if err != nil {
			return shared.ToolError("failed to list aggregates: %v", err), nil
		}

		aggs, err := aggregates.ExtractAggregates(page)
		if err != nil {
			return shared.ToolError("failed to extract aggregates: %v", err), nil
		}

		result := make([]map[string]any, 0, len(aggs))
		for _, agg := range aggs {
			result = append(result, map[string]any{
				"id":                agg.ID,
				"name":              agg.Name,
				"availability_zone": agg.AvailabilityZone,
				"hosts":             agg.Hosts,
				"metadata":          agg.Metadata,
				"created_at":        agg.CreatedAt,
				"updated_at":        agg.UpdatedAt,
			})
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
