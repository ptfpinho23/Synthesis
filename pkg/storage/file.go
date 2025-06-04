package storage

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/synthesis/orchestrator/pkg/api"
)

// FileStorage implements Storage interface using local filesystem
type FileStorage struct {
	dataDir string
}

// NewFileStorage creates a new file-based storage
func NewFileStorage(dataDir string) (*FileStorage, error) {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	
	// Create subdirectories for Kubernetes-compatible resources
	subdirs := []string{"pods", "deployments", "statefulsets", "services", "nodes"}
	for _, subdir := range subdirs {
		dir := filepath.Join(dataDir, subdir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create subdirectory %s: %w", subdir, err)
		}
	}
	
	return &FileStorage{
		dataDir: dataDir,
	}, nil
}

// Pod operations

func (fs *FileStorage) StorePod(pod *api.Pod) error {
	return fs.storeObject("pods", pod.Name, pod)
}

func (fs *FileStorage) GetPod(name string) (*api.Pod, error) {
	var pod api.Pod
	if err := fs.getObject("pods", name, &pod); err != nil {
		return nil, err
	}
	return &pod, nil
}

func (fs *FileStorage) ListPods() ([]*api.Pod, error) {
	files, err := fs.listFiles("pods")
	if err != nil {
		return nil, err
	}
	
	var pods []*api.Pod
	for _, file := range files {
		var pod api.Pod
		if err := fs.getObject("pods", file, &pod); err != nil {
			continue // Skip invalid files
		}
		pods = append(pods, &pod)
	}
	
	return pods, nil
}

func (fs *FileStorage) DeletePod(name string) error {
	return fs.deleteObject("pods", name)
}

// Deployment operations

func (fs *FileStorage) StoreDeployment(deployment *api.Deployment) error {
	return fs.storeObject("deployments", deployment.Name, deployment)
}

func (fs *FileStorage) GetDeployment(name string) (*api.Deployment, error) {
	var deployment api.Deployment
	if err := fs.getObject("deployments", name, &deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
}

func (fs *FileStorage) ListDeployments() ([]*api.Deployment, error) {
	files, err := fs.listFiles("deployments")
	if err != nil {
		return nil, err
	}
	
	var deployments []*api.Deployment
	for _, file := range files {
		var deployment api.Deployment
		if err := fs.getObject("deployments", file, &deployment); err != nil {
			continue // Skip invalid files
		}
		deployments = append(deployments, &deployment)
	}
	
	return deployments, nil
}

func (fs *FileStorage) DeleteDeployment(name string) error {
	return fs.deleteObject("deployments", name)
}

// StatefulSet operations

func (fs *FileStorage) StoreStatefulSet(statefulset *api.StatefulSet) error {
	return fs.storeObject("statefulsets", statefulset.Name, statefulset)
}

func (fs *FileStorage) GetStatefulSet(name string) (*api.StatefulSet, error) {
	var statefulset api.StatefulSet
	if err := fs.getObject("statefulsets", name, &statefulset); err != nil {
		return nil, err
	}
	return &statefulset, nil
}

func (fs *FileStorage) ListStatefulSets() ([]*api.StatefulSet, error) {
	files, err := fs.listFiles("statefulsets")
	if err != nil {
		return nil, err
	}
	
	var statefulsets []*api.StatefulSet
	for _, file := range files {
		var statefulset api.StatefulSet
		if err := fs.getObject("statefulsets", file, &statefulset); err != nil {
			continue // Skip invalid files
		}
		statefulsets = append(statefulsets, &statefulset)
	}
	
	return statefulsets, nil
}

func (fs *FileStorage) DeleteStatefulSet(name string) error {
	return fs.deleteObject("statefulsets", name)
}

// Service operations

func (fs *FileStorage) StoreService(service *api.Service) error {
	return fs.storeObject("services", service.Name, service)
}

func (fs *FileStorage) GetService(name string) (*api.Service, error) {
	var service api.Service
	if err := fs.getObject("services", name, &service); err != nil {
		return nil, err
	}
	return &service, nil
}

func (fs *FileStorage) ListServices() ([]*api.Service, error) {
	files, err := fs.listFiles("services")
	if err != nil {
		return nil, err
	}
	
	var services []*api.Service
	for _, file := range files {
		var service api.Service
		if err := fs.getObject("services", file, &service); err != nil {
			continue // Skip invalid files
		}
		services = append(services, &service)
	}
	
	return services, nil
}

func (fs *FileStorage) DeleteService(name string) error {
	return fs.deleteObject("services", name)
}

// Node operations

func (fs *FileStorage) StoreNode(node *api.Node) error {
	return fs.storeObject("nodes", node.Name, node)
}

func (fs *FileStorage) GetNode(name string) (*api.Node, error) {
	var node api.Node
	if err := fs.getObject("nodes", name, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

func (fs *FileStorage) ListNodes() ([]*api.Node, error) {
	files, err := fs.listFiles("nodes")
	if err != nil {
		return nil, err
	}
	
	var nodes []*api.Node
	for _, file := range files {
		var node api.Node
		if err := fs.getObject("nodes", file, &node); err != nil {
			continue // Skip invalid files
		}
		nodes = append(nodes, &node)
	}
	
	return nodes, nil
}

func (fs *FileStorage) DeleteNode(name string) error {
	return fs.deleteObject("nodes", name)
}

// Close closes the storage (no-op for file storage)
func (fs *FileStorage) Close() error {
	return nil
}

// Helper methods

func (fs *FileStorage) storeObject(category, name string, obj interface{}) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}
	
	filename := filepath.Join(fs.dataDir, category, name+".json")
	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	
	return nil
}

func (fs *FileStorage) getObject(category, name string, obj interface{}) error {
	filename := filepath.Join(fs.dataDir, category, name+".json")
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("object not found: %s", name)
		}
		return fmt.Errorf("failed to read file: %w", err)
	}
	
	if err := json.Unmarshal(data, obj); err != nil {
		return fmt.Errorf("failed to unmarshal object: %w", err)
	}
	
	return nil
}

func (fs *FileStorage) deleteObject(category, name string) error {
	filename := filepath.Join(fs.dataDir, category, name+".json")
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

func (fs *FileStorage) listFiles(category string) ([]string, error) {
	dir := filepath.Join(fs.dataDir, category)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}
	
	var names []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			name := file.Name()
			name = name[:len(name)-5] // Remove .json extension
			names = append(names, name)
		}
	}
	
	return names, nil
} 