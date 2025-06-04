package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"github.com/synthesis/orchestrator/pkg/api"
	"github.com/synthesis/orchestrator/pkg/runtime"
)

// DockerRuntime implements the ContainerRuntime interface using Docker
type DockerRuntime struct {
	client *client.Client
	config *runtime.RuntimeConfig
}

// NewDockerRuntime creates a new Docker runtime instance
func NewDockerRuntime(config *runtime.RuntimeConfig) (*DockerRuntime, error) {
	var cli *client.Client
	var err error

	if config.SocketPath != "" {
		cli, err = client.NewClientWithOpts(
			client.WithHost(config.SocketPath),
			client.WithAPIVersionNegotiation(),
		)
	} else {
		cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &DockerRuntime{
		client: cli,
		config: config,
	}, nil
}

// CreateContainer creates a new container from the given specification
func (d *DockerRuntime) CreateContainer(ctx context.Context, spec *api.Container, podName string) (*runtime.ContainerInfo, error) {
	// Convert API container spec to Docker config
	config := &container.Config{
		Image: spec.Image,
		Env:   d.convertEnvVars(spec.Env),
		Labels: map[string]string{
			"synthesis.pod":       podName,
			"synthesis.container": spec.Name,
		},
	}

	// Add default labels
	for k, v := range d.config.DefaultLabels {
		config.Labels[k] = v
	}

	// Convert ports
	exposedPorts, portBindings := d.convertPorts(spec.Ports)
	config.ExposedPorts = exposedPorts

	// Host config
	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		RestartPolicy: container.RestartPolicy{
			Name: d.convertRestartPolicy(spec.Resources.Limits),
		},
	}

	// Apply resource limits
	if spec.Resources.Limits != nil {
		if cpuLimit, ok := spec.Resources.Limits[api.ResourceCPU]; ok {
			if cpu, err := d.parseCPULimit(cpuLimit); err == nil {
				hostConfig.NanoCPUs = cpu
			}
		}
		if memLimit, ok := spec.Resources.Limits[api.ResourceMemory]; ok {
			if mem, err := d.parseMemoryLimit(memLimit); err == nil {
				hostConfig.Memory = mem
			}
		}
	}

	// Network config
	networkConfig := &network.NetworkingConfig{}
	if d.config.DefaultNetwork != "" {
		networkConfig.EndpointsConfig = map[string]*network.EndpointSettings{
			d.config.DefaultNetwork: {},
		}
	}

	// Create container
	containerName := fmt.Sprintf("%s-%s", podName, spec.Name)
	resp, err := d.client.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Return container info
	info, err := d.InspectContainer(ctx, resp.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect created container: %w", err)
	}

	return info, nil
}

// StartContainer starts a container
func (d *DockerRuntime) StartContainer(ctx context.Context, containerID string) error {
	return d.client.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
}

// StopContainer stops a container
func (d *DockerRuntime) StopContainer(ctx context.Context, containerID string, timeout int) error {
	timeoutDuration := time.Duration(timeout) * time.Second
	return d.client.ContainerStop(ctx, containerID, &timeoutDuration)
}

// RemoveContainer removes a container
func (d *DockerRuntime) RemoveContainer(ctx context.Context, containerID string) error {
	return d.client.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})
}

// RestartContainer restarts a container
func (d *DockerRuntime) RestartContainer(ctx context.Context, containerID string) error {
	timeout := time.Duration(d.config.Timeout) * time.Second
	return d.client.ContainerRestart(ctx, containerID, &timeout)
}

// InspectContainer returns detailed information about a container
func (d *DockerRuntime) InspectContainer(ctx context.Context, containerID string) (*runtime.ContainerInfo, error) {
	inspect, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	return d.convertContainerInfo(inspect), nil
}

// ListContainers lists containers based on filters
func (d *DockerRuntime) ListContainers(ctx context.Context, filters runtime.ContainerFilter) ([]*runtime.ContainerInfo, error) {
	// Convert filters
	dockerFilters := d.convertContainerFilters(filters)

	containers, err := d.client.ContainerList(ctx, types.ContainerListOptions{
		All:     true,
		Filters: dockerFilters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var result []*runtime.ContainerInfo
	for _, container := range containers {
		info := d.convertContainerSummary(container)
		result = append(result, info)
	}

	return result, nil
}

// GetContainerLogs retrieves container logs
func (d *DockerRuntime) GetContainerLogs(ctx context.Context, containerID string, opts runtime.LogOptions) (io.ReadCloser, error) {
	dockerOpts := types.ContainerLogsOptions{
		ShowStdout: opts.Stdout,
		ShowStderr: opts.Stderr,
		Follow:     opts.Follow,
		Since:      opts.Since,
		Until:      opts.Until,
		Timestamps: opts.Timestamps,
		Tail:       opts.Tail,
	}

	return d.client.ContainerLogs(ctx, containerID, dockerOpts)
}

// ExecContainer executes a command in a container
func (d *DockerRuntime) ExecContainer(ctx context.Context, containerID string, cmd []string) (*runtime.ExecResult, error) {
	execConfig := types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := d.client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create exec: %w", err)
	}

	attachResp, err := d.client.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer attachResp.Close()

	// Read output
	stdout, stderr, err := d.readExecOutput(attachResp.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read exec output: %w", err)
	}

	// Get exit code
	inspectResp, err := d.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect exec: %w", err)
	}

	return &runtime.ExecResult{
		ExitCode: inspectResp.ExitCode,
		Stdout:   stdout,
		Stderr:   stderr,
	}, nil
}

// PullImage pulls an image
func (d *DockerRuntime) PullImage(ctx context.Context, image string) error {
	_, err := d.client.ImagePull(ctx, image, types.ImagePullOptions{})
	return err
}

// RemoveImage removes an image
func (d *DockerRuntime) RemoveImage(ctx context.Context, image string) error {
	_, err := d.client.ImageRemove(ctx, image, types.ImageRemoveOptions{Force: true})
	return err
}

// ListImages lists available images
func (d *DockerRuntime) ListImages(ctx context.Context) ([]*runtime.ImageInfo, error) {
	images, err := d.client.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	var result []*runtime.ImageInfo
	for _, image := range images {
		info := &runtime.ImageInfo{
			ID:       image.ID,
			RepoTags: image.RepoTags,
			Size:     image.Size,
			Created:  image.Created,
			Labels:   image.Labels,
		}
		result = append(result, info)
	}

	return result, nil
}

// CreateNetwork creates a network
func (d *DockerRuntime) CreateNetwork(ctx context.Context, name string, opts runtime.NetworkOptions) (*runtime.NetworkInfo, error) {
	networkOpts := types.NetworkCreate{
		Driver:     opts.Driver,
		Internal:   opts.Internal,
		Attachable: opts.Attachable,
		Options:    opts.Options,
		Labels:     opts.Labels,
	}

	resp, err := d.client.NetworkCreate(ctx, name, networkOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}

	return &runtime.NetworkInfo{
		ID:      resp.ID,
		Name:    name,
		Driver:  opts.Driver,
		Options: opts.Options,
		Labels:  opts.Labels,
	}, nil
}

// RemoveNetwork removes a network
func (d *DockerRuntime) RemoveNetwork(ctx context.Context, networkID string) error {
	return d.client.NetworkRemove(ctx, networkID)
}

// ConnectContainer connects a container to a network
func (d *DockerRuntime) ConnectContainer(ctx context.Context, containerID, networkID string) error {
	return d.client.NetworkConnect(ctx, networkID, containerID, nil)
}

// DisconnectContainer disconnects a container from a network
func (d *DockerRuntime) DisconnectContainer(ctx context.Context, containerID, networkID string) error {
	return d.client.NetworkDisconnect(ctx, networkID, containerID, false)
}

// GetContainerStats retrieves container statistics
func (d *DockerRuntime) GetContainerStats(ctx context.Context, containerID string) (*runtime.ContainerStats, error) {
	stats, err := d.client.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer stats.Body.Close()

	// Parse stats (simplified implementation)
	// In a real implementation, you would properly decode the JSON stream
	return &runtime.ContainerStats{
		ContainerID: containerID,
		Read:        time.Now().Unix(),
		// TODO: Parse actual stats from JSON stream
	}, nil
}

// GetSystemInfo retrieves system information
func (d *DockerRuntime) GetSystemInfo(ctx context.Context) (*runtime.SystemInfo, error) {
	info, err := d.client.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	return &runtime.SystemInfo{
		ContainerRuntime:        "docker",
		RuntimeVersion:          info.ServerVersion,
		KernelVersion:           info.KernelVersion,
		OperatingSystem:         info.OperatingSystem,
		Architecture:            info.Architecture,
		NCPU:                    info.NCPU,
		MemTotal:                info.MemTotal,
		DockerRootDir:           info.DockerRootDir,
		HTTPProxy:               info.HTTPProxy,
		HTTPSProxy:              info.HTTPSProxy,
		NoProxy:                 info.NoProxy,
	}, nil
}

// HealthCheck checks if the runtime is healthy
func (d *DockerRuntime) HealthCheck(ctx context.Context) error {
	_, err := d.client.Ping(ctx)
	return err
}

// Helper functions

func (d *DockerRuntime) convertEnvVars(envVars []api.EnvVar) []string {
	var result []string
	for _, env := range envVars {
		result = append(result, fmt.Sprintf("%s=%s", env.Name, env.Value))
	}
	return result
}

func (d *DockerRuntime) convertPorts(ports []api.ContainerPort) (nat.PortSet, nat.PortMap) {
	exposedPorts := make(nat.PortSet)
	portBindings := make(nat.PortMap)

	for _, port := range ports {
		natPort := nat.Port(fmt.Sprintf("%d/%s", port.ContainerPort, strings.ToLower(string(port.Protocol))))
		exposedPorts[natPort] = struct{}{}

		// For simplicity, bind to random host ports
		portBindings[natPort] = []nat.PortBinding{
			{
				HostIP: "0.0.0.0",
			},
		}
	}

	return exposedPorts, portBindings
}

func (d *DockerRuntime) convertRestartPolicy(limits api.ResourceList) string {
	// Simplified restart policy conversion
	return "unless-stopped"
}

func (d *DockerRuntime) parseCPULimit(cpuLimit string) (int64, error) {
	// Parse CPU limit (e.g., "1", "0.5", "1000m")
	if strings.HasSuffix(cpuLimit, "m") {
		// Millicore format
		milliCPU, err := strconv.ParseInt(cpuLimit[:len(cpuLimit)-1], 10, 64)
		if err != nil {
			return 0, err
		}
		return milliCPU * 1000000, nil // Convert to nanoseconds
	}

	// Standard format
	cpu, err := strconv.ParseFloat(cpuLimit, 64)
	if err != nil {
		return 0, err
	}
	return int64(cpu * 1000000000), nil // Convert to nanoseconds
}

func (d *DockerRuntime) parseMemoryLimit(memLimit string) (int64, error) {
	// Parse memory limit (e.g., "1Gi", "512Mi", "1024")
	multiplier := int64(1)
	
	if strings.HasSuffix(memLimit, "Ki") {
		multiplier = 1024
		memLimit = memLimit[:len(memLimit)-2]
	} else if strings.HasSuffix(memLimit, "Mi") {
		multiplier = 1024 * 1024
		memLimit = memLimit[:len(memLimit)-2]
	} else if strings.HasSuffix(memLimit, "Gi") {
		multiplier = 1024 * 1024 * 1024
		memLimit = memLimit[:len(memLimit)-2]
	}

	mem, err := strconv.ParseInt(memLimit, 10, 64)
	if err != nil {
		return 0, err
	}
	return mem * multiplier, nil
}

func (d *DockerRuntime) convertContainerInfo(inspect types.ContainerJSON) *runtime.ContainerInfo {
	var ports []runtime.PortMapping
	for port, bindings := range inspect.NetworkSettings.Ports {
		for _, binding := range bindings {
			hostPort, _ := strconv.Atoi(binding.HostPort)
			containerPort, _ := strconv.Atoi(port.Port())
			ports = append(ports, runtime.PortMapping{
				ContainerPort: int32(containerPort),
				HostPort:      int32(hostPort),
				Protocol:      port.Proto(),
				HostIP:        binding.HostIP,
			})
		}
	}

	return &runtime.ContainerInfo{
		ID:      inspect.ID,
		Name:    strings.TrimPrefix(inspect.Name, "/"),
		Image:   inspect.Config.Image,
		Status:  runtime.ContainerStatus(inspect.State.Status),
		Created: inspect.Created.Unix(),
		Started: inspect.State.StartedAt.Unix(),
		Labels:  inspect.Config.Labels,
		Ports:   ports,
	}
}

func (d *DockerRuntime) convertContainerSummary(container types.Container) *runtime.ContainerInfo {
	var ports []runtime.PortMapping
	for _, port := range container.Ports {
		ports = append(ports, runtime.PortMapping{
			ContainerPort: int32(port.PrivatePort),
			HostPort:      int32(port.PublicPort),
			Protocol:      port.Type,
			HostIP:        port.IP,
		})
	}

	return &runtime.ContainerInfo{
		ID:      container.ID,
		Name:    strings.Join(container.Names, ","),
		Image:   container.Image,
		Status:  runtime.ContainerStatus(container.Status),
		Created: container.Created,
		Labels:  container.Labels,
		Ports:   ports,
	}
}

func (d *DockerRuntime) convertContainerFilters(filters runtime.ContainerFilter) map[string][]string {
	dockerFilters := make(map[string][]string)

	// Convert labels
	for k, v := range filters.Labels {
		dockerFilters["label"] = append(dockerFilters["label"], fmt.Sprintf("%s=%s", k, v))
	}

	// Convert names
	if len(filters.Names) > 0 {
		dockerFilters["name"] = filters.Names
	}

	// Convert status
	for _, status := range filters.Status {
		dockerFilters["status"] = append(dockerFilters["status"], string(status))
	}

	return dockerFilters
}

func (d *DockerRuntime) readExecOutput(reader io.Reader) (string, string, error) {
	// This is a simplified implementation
	// In reality, you'd need to properly demultiplex stdout and stderr
	// from the Docker attach stream
	
	output := make([]byte, 4096)
	n, err := reader.Read(output)
	if err != nil && err != io.EOF {
		return "", "", err
	}
	
	return string(output[:n]), "", nil
} 