package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/synthesis/orchestrator/pkg/api"
)

var (
	serverURL string
	output    string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "synthesis-cli",
		Short: "CLI for Synthesis container orchestrator",
		Long:  "Command line interface for managing workloads and services in Synthesis",
	}

	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:8080", "Synthesis server URL")
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "table", "Output format: table, json, yaml")

	var workloadCmd = &cobra.Command{
		Use:   "workload",
		Short: "Manage workloads",
		Aliases: []string{"workloads", "w"},
	}

	workloadCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List workloads",
			Run:   listWorkloads,
		},
		&cobra.Command{
			Use:   "get [name]",
			Short: "Get workload details",
			Args:  cobra.ExactArgs(1),
			Run:   getWorkload,
		},
		&cobra.Command{
			Use:   "create [file]",
			Short: "Create workload from file",
			Args:  cobra.ExactArgs(1),
			Run:   createWorkload,
		},
		&cobra.Command{
			Use:   "delete [name]",
			Short: "Delete workload",
			Args:  cobra.ExactArgs(1),
			Run:   deleteWorkload,
		},
		&cobra.Command{
			Use:   "scale [name] [replicas]",
			Short: "Scale workload",
			Args:  cobra.ExactArgs(2),
			Run:   scaleWorkload,
		},
	)

	var serviceCmd = &cobra.Command{
		Use:   "service",
		Short: "Manage services",
		Aliases: []string{"services", "svc"},
	}

	serviceCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List services",
			Run:   listServices,
		},
		&cobra.Command{
			Use:   "get [name]",
			Short: "Get service details",
			Args:  cobra.ExactArgs(1),
			Run:   getService,
		},
		&cobra.Command{
			Use:   "create [file]",
			Short: "Create service from file",
			Args:  cobra.ExactArgs(1),
			Run:   createService,
		},
		&cobra.Command{
			Use:   "delete [name]",
			Short: "Delete service",
			Args:  cobra.ExactArgs(1),
			Run:   deleteService,
		},
	)

	var nodeCmd = &cobra.Command{
		Use:   "node",
		Short: "Manage nodes",
		Aliases: []string{"nodes"},
	}

	nodeCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List nodes",
			Run:   listNodes,
		},
		&cobra.Command{
			Use:   "get [name]",
			Short: "Get node details",
			Args:  cobra.ExactArgs(1),
			Run:   getNode,
		},
	)

	var containerCmd = &cobra.Command{
		Use:   "container",
		Short: "Manage containers",
		Aliases: []string{"containers"},
	}

	containerCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List containers",
			Run:   listContainers,
		},
		&cobra.Command{
			Use:   "logs [id]",
			Short: "Get container logs",
			Args:  cobra.ExactArgs(1),
			Run:   getContainerLogs,
		},
		&cobra.Command{
			Use:   "exec [id] [command...]",
			Short: "Execute command in container",
			Args:  cobra.MinimumNArgs(2),
			Run:   execContainer,
		},
	)

	var systemCmd = &cobra.Command{
		Use:   "system",
		Short: "System information and management",
	}

	systemCmd.AddCommand(
		&cobra.Command{
			Use:   "info",
			Short: "Get system information",
			Run:   getSystemInfo,
		},
		&cobra.Command{
			Use:   "health",
			Short: "Check system health",
			Run:   getSystemHealth,
		},
	)

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Synthesis CLI v0.1.0")
		},
	}

	rootCmd.AddCommand(workloadCmd)
	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(nodeCmd)
	rootCmd.AddCommand(containerCmd)
	rootCmd.AddCommand(systemCmd)
	rootCmd.AddCommand(versionCmd)

	// Add kubectl-style commands
	var applyCmd = &cobra.Command{
		Use:   "apply -f [file]",
		Short: "Apply configuration from file",
		Run:   applyConfig,
	}
	applyCmd.Flags().StringP("file", "f", "", "Filename to apply")
	applyCmd.MarkFlagRequired("file")

	var getCmd = &cobra.Command{
		Use:   "get [resource] [name]",
		Short: "Get resources",
		Args:  cobra.MinimumNArgs(1),
		Run:   getResource,
	}

	var deleteCmd = &cobra.Command{
		Use:   "delete [resource] [name]",
		Short: "Delete resources",
		Args:  cobra.MinimumNArgs(1),
		Run:   deleteResource,
	}

	var scaleCmd = &cobra.Command{
		Use:   "scale [resource] [name] --replicas=[count]",
		Short: "Scale resources",
		Args:  cobra.ExactArgs(2),
		Run:   scaleResource,
	}
	scaleCmd.Flags().Int("replicas", 1, "Number of replicas")
	scaleCmd.MarkFlagRequired("replicas")

	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(scaleCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func listWorkloads(cmd *cobra.Command, args []string) {
	resp, err := makeRequest("GET", "/api/v1/workloads", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var result struct {
		Items []*api.SynthesisWorkload `json:"items"`
		Count int             `json:"count"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	switch output {
	case "json":
		fmt.Println(string(resp))
	case "yaml":
		data, _ := yaml.Marshal(result.Items)
		fmt.Println(string(data))
	default:
		printWorkloadsTable(result.Items)
	}
}

func getWorkload(cmd *cobra.Command, args []string) {
	name := args[0]
	resp, err := makeRequest("GET", fmt.Sprintf("/api/v1/workloads/%s", name), nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	switch output {
	case "json":
		fmt.Println(string(resp))
	case "yaml":
		var workload api.SynthesisWorkload
		json.Unmarshal(resp, &workload)
		data, _ := yaml.Marshal(workload)
		fmt.Println(string(data))
	default:
		var workload api.SynthesisWorkload
		json.Unmarshal(resp, &workload)
		printWorkloadsTable([]*api.SynthesisWorkload{&workload})
	}
}

func createWorkload(cmd *cobra.Command, args []string) {
	filename := args[0]
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	var workload api.SynthesisWorkload
	if strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml") {
		err = yaml.Unmarshal(data, &workload)
	} else {
		err = json.Unmarshal(data, &workload)
	}

	if err != nil {
		fmt.Printf("Error parsing file: %v\n", err)
		return
	}

	_, err = makeRequest("POST", "/api/v1/workloads", &workload)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Workload %s created successfully\n", workload.ObjectMeta.Name)
}

func deleteWorkload(cmd *cobra.Command, args []string) {
	name := args[0]
	_, err := makeRequest("DELETE", fmt.Sprintf("/api/v1/workloads/%s", name), nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Workload %s deleted successfully\n", name)
}

func scaleWorkload(cmd *cobra.Command, args []string) {
	name := args[0]
	replicas := args[1]
	
	// Get current workload
	resp, err := makeRequest("GET", fmt.Sprintf("/api/v1/workloads/%s", name), nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	var workload api.SynthesisWorkload
	if err := json.Unmarshal(resp, &workload); err != nil {
		fmt.Printf("Error parsing workload: %v\n", err)
		return
	}
	
	// Parse replicas
	var replicaCount int32
	fmt.Sscanf(replicas, "%d", &replicaCount)
	workload.Spec.Replicas = replicaCount
	
	// Update workload
	_, err = makeRequest("PUT", fmt.Sprintf("/api/v1/workloads/%s", name), &workload)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Printf("Workload %s scaled to %d replicas\n", name, replicaCount)
}

func listServices(cmd *cobra.Command, args []string) {
	resp, err := makeRequest("GET", "/api/v1/services", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var result struct {
		Items []*api.Service `json:"items"`
		Count int            `json:"count"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	switch output {
	case "json":
		fmt.Println(string(resp))
	case "yaml":
		data, _ := yaml.Marshal(result.Items)
		fmt.Println(string(data))
	default:
		printServicesTable(result.Items)
	}
}

func getService(cmd *cobra.Command, args []string) {
	name := args[0]
	resp, err := makeRequest("GET", fmt.Sprintf("/api/v1/services/%s", name), nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	switch output {
	case "json":
		fmt.Println(string(resp))
	case "yaml":
		var service api.Service
		json.Unmarshal(resp, &service)
		data, _ := yaml.Marshal(service)
		fmt.Println(string(data))
	default:
		var service api.Service
		json.Unmarshal(resp, &service)
		printServicesTable([]*api.Service{&service})
	}
}

func createService(cmd *cobra.Command, args []string) {
	filename := args[0]
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	var service api.Service
	if strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml") {
		err = yaml.Unmarshal(data, &service)
	} else {
		err = json.Unmarshal(data, &service)
	}

	if err != nil {
		fmt.Printf("Error parsing file: %v\n", err)
		return
	}

	_, err = makeRequest("POST", "/api/v1/services", &service)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Service %s created successfully\n", service.ObjectMeta.Name)
}

func deleteService(cmd *cobra.Command, args []string) {
	name := args[0]
	_, err := makeRequest("DELETE", fmt.Sprintf("/api/v1/services/%s", name), nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Service %s deleted successfully\n", name)
}

func listNodes(cmd *cobra.Command, args []string) {
	resp, err := makeRequest("GET", "/api/v1/nodes", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var result struct {
		Items []*api.Node `json:"items"`
		Count int         `json:"count"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	switch output {
	case "json":
		fmt.Println(string(resp))
	case "yaml":
		data, _ := yaml.Marshal(result.Items)
		fmt.Println(string(data))
	default:
		printNodesTable(result.Items)
	}
}

func getNode(cmd *cobra.Command, args []string) {
	name := args[0]
	resp, err := makeRequest("GET", fmt.Sprintf("/api/v1/nodes/%s", name), nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	switch output {
	case "json":
		fmt.Println(string(resp))
	case "yaml":
		var node api.Node
		json.Unmarshal(resp, &node)
		data, _ := yaml.Marshal(node)
		fmt.Println(string(data))
	default:
		var node api.Node
		json.Unmarshal(resp, &node)
		printNodesTable([]*api.Node{&node})
	}
}

func listContainers(cmd *cobra.Command, args []string) {
	resp, err := makeRequest("GET", "/api/v1/containers", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println(string(resp))
}

func getContainerLogs(cmd *cobra.Command, args []string) {
	containerID := args[0]
	resp, err := makeRequest("GET", fmt.Sprintf("/api/v1/containers/%s/logs", containerID), nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println(string(resp))
}

func execContainer(cmd *cobra.Command, args []string) {
	containerID := args[0]
	command := args[1:]

	req := map[string]interface{}{
		"command": command,
	}

	resp, err := makeRequest("POST", fmt.Sprintf("/api/v1/containers/%s/exec", containerID), req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println(string(resp))
}

func getSystemInfo(cmd *cobra.Command, args []string) {
	resp, err := makeRequest("GET", "/api/v1/system/info", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	switch output {
	case "json":
		fmt.Println(string(resp))
	case "yaml":
		var info map[string]interface{}
		json.Unmarshal(resp, &info)
		data, _ := yaml.Marshal(info)
		fmt.Println(string(data))
	default:
		fmt.Println(string(resp))
	}
}

func getSystemHealth(cmd *cobra.Command, args []string) {
	resp, err := makeRequest("GET", "/health", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println(string(resp))
}

func makeRequest(method, path string, body interface{}) ([]byte, error) {
	var reqBody []byte
	var err error
	var contentType string

	if body != nil {
		switch v := body.(type) {
		case []byte:
			// Raw bytes (e.g., YAML data)
			reqBody = v
			contentType = "application/yaml"
		default:
			// JSON object
			reqBody, err = json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			contentType = "application/json"
		}
	}

	req, err := http.NewRequest(method, serverURL+path, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func printWorkloadsTable(workloads []*api.SynthesisWorkload) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tREPLICAS\tREADY\tAVAILABLE\tAGE")

	for _, workload := range workloads {
		age := time.Since(workload.ObjectMeta.CreationTimestamp.Time).Truncate(time.Second)
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n",
			workload.ObjectMeta.Name,
			workload.Spec.Replicas,
			workload.Status.ReadyReplicas,
			workload.Status.AvailableReplicas,
			age)
	}

	w.Flush()
}

func printServicesTable(services []*api.Service) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tPORTS\tAGE")

	for _, service := range services {
		age := time.Since(service.ObjectMeta.CreationTimestamp.Time).Truncate(time.Second)
		var ports []string
		for _, port := range service.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
		}
		portStr := strings.Join(ports, ",")

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			service.ObjectMeta.Name,
			service.Spec.Type,
			portStr,
			age)
	}

	w.Flush()
}

func printNodesTable(nodes []*api.Node) {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tROLES\tAGE\tVERSION")
	
	for _, node := range nodes {
		age := time.Since(node.CreationTimestamp.Time).Round(time.Second)
		
		// Determine status from conditions
		status := "Unknown"
		for _, condition := range node.Status.Conditions {
			if condition.Type == api.NodeReady && condition.Status == api.ConditionTrue {
				status = "Ready"
				break
			}
		}
		
		// Get roles from labels (standard Kubernetes approach)
		roles := []string{}
		for key := range node.Labels {
			if strings.HasPrefix(key, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(key, "node-role.kubernetes.io/")
				if role != "" {
					roles = append(roles, role)
				}
			}
		}
		if len(roles) == 0 {
			roles = append(roles, "<none>")
		}
		
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\n",
			node.Name,
			status,
			strings.Join(roles, ","),
			age,
			node.Status.NodeInfo.KubeletVersion,
		)
	}
	
	w.Flush()
}

// kubectl-style command implementations
func applyConfig(cmd *cobra.Command, args []string) {
	filename, _ := cmd.Flags().GetString("file")
	
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	// Parse YAML to determine resource type
	var resource map[string]interface{}
	if err := yaml.Unmarshal(data, &resource); err != nil {
		fmt.Printf("Error parsing YAML: %v\n", err)
		return
	}

	kind, ok := resource["kind"].(string)
	if !ok {
		fmt.Printf("Error: missing or invalid 'kind' field\n")
		return
	}

	// Route to appropriate create function based on kind
	switch strings.ToLower(kind) {
	case "deployment", "statefulset":
		createWorkloadFromData(data)
	case "service":
		createServiceFromData(data)
	default:
		fmt.Printf("Error: unsupported resource kind '%s'\n", kind)
	}
}

func getResource(cmd *cobra.Command, args []string) {
	resourceType := strings.ToLower(args[0])
	
	switch resourceType {
	case "pods", "pod":
		if len(args) > 1 {
			// Get specific pod
			resp, err := makeRequest("GET", fmt.Sprintf("/api/v1/pods/%s", args[1]), nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		} else {
			// List pods
			resp, err := makeRequest("GET", "/api/v1/pods", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		}
	case "deployments", "deployment", "deploy":
		if len(args) > 1 {
			// Get specific deployment
			resp, err := makeRequest("GET", fmt.Sprintf("/apis/apps/v1/deployments/%s", args[1]), nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		} else {
			// List deployments
			resp, err := makeRequest("GET", "/apis/apps/v1/deployments", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		}
	case "statefulsets", "statefulset", "sts":
		if len(args) > 1 {
			// Get specific statefulset
			resp, err := makeRequest("GET", fmt.Sprintf("/apis/apps/v1/statefulsets/%s", args[1]), nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		} else {
			// List statefulsets
			resp, err := makeRequest("GET", "/apis/apps/v1/statefulsets", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		}
	case "services", "service", "svc":
		if len(args) > 1 {
			// Get specific service
			resp, err := makeRequest("GET", fmt.Sprintf("/api/v1/services/%s", args[1]), nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		} else {
			// List services
			resp, err := makeRequest("GET", "/api/v1/services", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		}
	case "nodes", "node":
		if len(args) > 1 {
			// Get specific node
			resp, err := makeRequest("GET", fmt.Sprintf("/api/v1/nodes/%s", args[1]), nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		} else {
			// List nodes
			resp, err := makeRequest("GET", "/api/v1/nodes", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(string(resp))
		}
	default:
		fmt.Printf("Error: unsupported resource type '%s'\n", resourceType)
	}
}

func deleteResource(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		fmt.Printf("Error: resource type and name required\n")
		return
	}
	
	resourceType := strings.ToLower(args[0])
	name := args[1]
	
	switch resourceType {
	case "deployment", "deploy":
		deleteWorkload(cmd, []string{name})
	case "service", "svc":
		deleteService(cmd, []string{name})
	default:
		fmt.Printf("Error: unsupported resource type '%s'\n", resourceType)
	}
}

func scaleResource(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		fmt.Printf("Error: resource type and name required\n")
		return
	}
	
	resourceType := strings.ToLower(args[0])
	name := args[1]
	replicas, _ := cmd.Flags().GetInt("replicas")
	
	switch resourceType {
	case "deployment", "deploy":
		scaleWorkload(cmd, []string{name, fmt.Sprintf("%d", replicas)})
	default:
		fmt.Printf("Error: scaling not supported for resource type '%s'\n", resourceType)
	}
}

func createWorkloadFromData(data []byte) {
	// Parse YAML to determine the specific kind
	var resource map[string]interface{}
	if err := yaml.Unmarshal(data, &resource); err != nil {
		fmt.Printf("Error parsing YAML: %v\n", err)
		return
	}

	kind, ok := resource["kind"].(string)
	if !ok {
		fmt.Printf("Error: missing or invalid 'kind' field\n")
		return
	}

	var endpoint string
	switch strings.ToLower(kind) {
	case "deployment":
		endpoint = "/apis/apps/v1/deployments"
	case "statefulset":
		endpoint = "/apis/apps/v1/statefulsets"
	case "pod":
		endpoint = "/api/v1/pods"
	default:
		fmt.Printf("Error: unsupported workload kind '%s'\n", kind)
		return
	}

	resp, err := makeRequest("POST", endpoint, data)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Parse response to get the name
	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	if metadata, ok := result["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			fmt.Printf("%s/%s created\n", strings.ToLower(kind), name)
			return
		}
	}

	fmt.Printf("%s created\n", strings.ToLower(kind))
}

func createServiceFromData(data []byte) {
	resp, err := makeRequest("POST", "/api/v1/services", data)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Parse response to get the name
	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	if metadata, ok := result["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			fmt.Printf("service/%s created\n", name)
			return
		}
	}

	fmt.Printf("service created\n")
} 