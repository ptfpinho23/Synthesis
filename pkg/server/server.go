package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"

	"github.com/synthesis/orchestrator/pkg/api"
	"github.com/synthesis/orchestrator/pkg/runtime"
	"github.com/synthesis/orchestrator/pkg/storage"
)

// Config represents server configuration
type Config struct {
	ListenAddr string               `json:"listen_addr"`
	Debug      bool                 `json:"debug"`
	DataDir    string               `json:"data_dir"`
	Runtime    runtime.RuntimeConfig `json:"runtime"`
}

// Server represents the main orchestrator server
type Server struct {
	config  *Config
	runtime runtime.ContainerRuntime
	storage storage.Storage
	
	// Controllers
	workloadController *WorkloadController
	serviceController  *ServiceController
	
	// State management (Kubernetes-compatible resources)
	pods         map[string]*api.Pod
	deployments  map[string]*api.Deployment
	statefulsets map[string]*api.StatefulSet
	services     map[string]*api.Service
	nodes        map[string]*api.Node
	mutex        sync.RWMutex
}

// NewServer creates a new orchestrator server
func NewServer(config *Config, runtime runtime.ContainerRuntime) (*Server, error) {
	// Initialize storage
	store, err := storage.NewFileStorage(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	server := &Server{
		config:       config,
		runtime:      runtime,
		storage:      store,
		pods:         make(map[string]*api.Pod),
		deployments:  make(map[string]*api.Deployment),
		statefulsets: make(map[string]*api.StatefulSet),
		services:     make(map[string]*api.Service),
		nodes:        make(map[string]*api.Node),
	}

	// Initialize controllers
	server.workloadController = NewWorkloadController(server)
	server.serviceController = NewServiceController(server)

	// Load existing state
	if err := server.loadState(); err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	return server, nil
}

// SetupRoutes configures HTTP routes with Kubernetes-compatible API paths
func (s *Server) SetupRoutes(router *mux.Router) {
	// Health endpoints
	router.HandleFunc("/health", s.healthHandler).Methods("GET")
	router.HandleFunc("/healthz", s.healthHandler).Methods("GET") // K8s style
	router.HandleFunc("/ready", s.readyHandler).Methods("GET")
	router.HandleFunc("/readyz", s.readyHandler).Methods("GET") // K8s style
	
	// Kubernetes-compatible API paths
	
	// Core API (v1) - Pods and Services
	coreAPI := router.PathPrefix("/api/v1").Subrouter()
	
	// Pod endpoints
	coreAPI.HandleFunc("/pods", s.listPodsHandler).Methods("GET")
	coreAPI.HandleFunc("/pods", s.createPodHandler).Methods("POST")
	coreAPI.HandleFunc("/pods/{name}", s.getPodHandler).Methods("GET")
	coreAPI.HandleFunc("/pods/{name}", s.updatePodHandler).Methods("PUT")
	coreAPI.HandleFunc("/pods/{name}", s.deletePodHandler).Methods("DELETE")
	
	// Service endpoints
	coreAPI.HandleFunc("/services", s.listServicesHandler).Methods("GET")
	coreAPI.HandleFunc("/services", s.createServiceHandler).Methods("POST")
	coreAPI.HandleFunc("/services/{name}", s.getServiceHandler).Methods("GET")
	coreAPI.HandleFunc("/services/{name}", s.updateServiceHandler).Methods("PUT")
	coreAPI.HandleFunc("/services/{name}", s.deleteServiceHandler).Methods("DELETE")
	
	// Generic workload endpoints
	coreAPI.HandleFunc("/workloads", s.listAllWorkloadsHandler).Methods("GET")
	coreAPI.HandleFunc("/workloads/{name}", s.getGenericWorkloadHandler).Methods("GET")
	coreAPI.HandleFunc("/workloads/{name}", s.updateGenericWorkloadHandler).Methods("PUT")
	coreAPI.HandleFunc("/workloads/{name}", s.deleteGenericWorkloadHandler).Methods("DELETE")
	coreAPI.HandleFunc("/workloads", s.createGenericWorkloadHandler).Methods("POST")
	
	// Container management endpoints
	coreAPI.HandleFunc("/containers", s.listContainersHandler).Methods("GET")
	coreAPI.HandleFunc("/containers/{id}/logs", s.getContainerLogsHandler).Methods("GET")
	coreAPI.HandleFunc("/containers/{id}/exec", s.execContainerHandler).Methods("POST")
	
	// System endpoints
	coreAPI.HandleFunc("/system/info", s.systemInfoHandler).Methods("GET")
	
	// Node endpoints
	coreAPI.HandleFunc("/nodes", s.listNodesHandler).Methods("GET")
	coreAPI.HandleFunc("/nodes/{name}", s.getNodeHandler).Methods("GET")
	
	// Apps API (v1) - Deployments and StatefulSets
	appsAPI := router.PathPrefix("/apis/apps/v1").Subrouter()
	
	// Deployment endpoints
	appsAPI.HandleFunc("/deployments", s.listDeploymentsHandler).Methods("GET")
	appsAPI.HandleFunc("/deployments", s.createDeploymentHandler).Methods("POST")
	appsAPI.HandleFunc("/deployments/{name}", s.getDeploymentHandler).Methods("GET")
	appsAPI.HandleFunc("/deployments/{name}", s.updateDeploymentHandler).Methods("PUT")
	appsAPI.HandleFunc("/deployments/{name}", s.deleteDeploymentHandler).Methods("DELETE")
	appsAPI.HandleFunc("/deployments/{name}/scale", s.scaleDeploymentHandler).Methods("PUT")
	
	// StatefulSet endpoints
	appsAPI.HandleFunc("/statefulsets", s.listStatefulSetsHandler).Methods("GET")
	appsAPI.HandleFunc("/statefulsets", s.createStatefulSetHandler).Methods("POST")
	appsAPI.HandleFunc("/statefulsets/{name}", s.getStatefulSetHandler).Methods("GET")
	appsAPI.HandleFunc("/statefulsets/{name}", s.updateStatefulSetHandler).Methods("PUT")
	appsAPI.HandleFunc("/statefulsets/{name}", s.deleteStatefulSetHandler).Methods("DELETE")
	appsAPI.HandleFunc("/statefulsets/{name}/scale", s.scaleStatefulSetHandler).Methods("PUT")
	
	// Generic manifest endpoint (auto-detect resource type)
	router.HandleFunc("/apply", s.applyManifestHandler).Methods("POST")
}

// StartControllers starts background controllers
func (s *Server) StartControllers(ctx context.Context) {
	log.Println("Starting orchestration controllers...")
	
	go s.workloadController.Run(ctx)
	go s.serviceController.Run(ctx)
	go s.runNodeController(ctx)
	
	log.Println("Controllers started")
}

// Health handlers
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"version":   "0.1.0",
		"runtime":   "containerd",
		"k8s_compatible": true,
	}
	
	// Check runtime health
	if err := s.runtime.HealthCheck(r.Context()); err != nil {
		health["status"] = "unhealthy"
		health["runtime_error"] = err.Error()
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	s.writeJSON(w, health)
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	ready := map[string]interface{}{
		"ready":     true,
		"timestamp": time.Now().Unix(),
	}
	s.writeJSON(w, ready)
}

// Pod handlers

func (s *Server) listPodsHandler(w http.ResponseWriter, r *http.Request) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	var pods []*api.Pod
	for _, pod := range s.pods {
		pods = append(pods, pod)
	}
	
	s.writeJSON(w, map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PodList",
		"items":      pods,
	})
}

func (s *Server) createPodHandler(w http.ResponseWriter, r *http.Request) {
	var pod api.Pod
	if err := s.decodeManifest(r, &pod); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	// Set default values
	s.setDefaultsForPod(&pod)
	
	// Store pod
	s.mutex.Lock()
	s.pods[pod.Name] = &pod
	s.mutex.Unlock()
	
	// Persist to storage
	if err := s.storage.StorePod(&pod); err != nil {
		log.Printf("Failed to persist pod: %v", err)
	}
	
	w.WriteHeader(http.StatusCreated)
	s.writeJSON(w, &pod)
}

func (s *Server) getPodHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.RLock()
	pod, exists := s.pods[name]
	s.mutex.RUnlock()
	
	if !exists {
		s.writeError(w, http.StatusNotFound, "Pod not found", nil)
		return
	}
	
	s.writeJSON(w, pod)
}

func (s *Server) updatePodHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	var pod api.Pod
	if err := s.decodeManifest(r, &pod); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	pod.Name = name
	s.setDefaultsForPod(&pod)
	
	s.mutex.Lock()
	s.pods[name] = &pod
	s.mutex.Unlock()
	
	if err := s.storage.StorePod(&pod); err != nil {
		log.Printf("Failed to persist pod: %v", err)
	}
	
	s.writeJSON(w, &pod)
}

func (s *Server) deletePodHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.Lock()
	delete(s.pods, name)
	s.mutex.Unlock()
	
	if err := s.storage.DeletePod(name); err != nil {
		log.Printf("Failed to delete pod from storage: %v", err)
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// Deployment handlers

func (s *Server) listDeploymentsHandler(w http.ResponseWriter, r *http.Request) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	var deployments []*api.Deployment
	for _, deployment := range s.deployments {
		deployments = append(deployments, deployment)
	}
	
	s.writeJSON(w, map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "DeploymentList",
		"items":      deployments,
	})
}

func (s *Server) createDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	var deployment api.Deployment
	if err := s.decodeManifest(r, &deployment); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	s.setDefaultsForDeployment(&deployment)
	
	s.mutex.Lock()
	s.deployments[deployment.Name] = &deployment
	s.mutex.Unlock()
	
	if err := s.storage.StoreDeployment(&deployment); err != nil {
		log.Printf("Failed to persist deployment: %v", err)
	}
	
	w.WriteHeader(http.StatusCreated)
	s.writeJSON(w, &deployment)
}

func (s *Server) getDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.RLock()
	deployment, exists := s.deployments[name]
	s.mutex.RUnlock()
	
	if !exists {
		s.writeError(w, http.StatusNotFound, "Deployment not found", nil)
		return
	}
	
	s.writeJSON(w, deployment)
}

func (s *Server) updateDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	var deployment api.Deployment
	if err := s.decodeManifest(r, &deployment); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	deployment.Name = name
	s.setDefaultsForDeployment(&deployment)
	
	s.mutex.Lock()
	s.deployments[name] = &deployment
	s.mutex.Unlock()
	
	if err := s.storage.StoreDeployment(&deployment); err != nil {
		log.Printf("Failed to persist deployment: %v", err)
	}
	
	s.writeJSON(w, &deployment)
}

func (s *Server) deleteDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.Lock()
	delete(s.deployments, name)
	s.mutex.Unlock()
	
	if err := s.storage.DeleteDeployment(name); err != nil {
		log.Printf("Failed to delete deployment from storage: %v", err)
	}
	
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) scaleDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	var scaleReq struct {
		Spec struct {
			Replicas int32 `json:"replicas"`
		} `json:"spec"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&scaleReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid scale request", err)
		return
	}
	
	s.mutex.Lock()
	if deployment, exists := s.deployments[name]; exists {
		deployment.Spec.Replicas = &scaleReq.Spec.Replicas
	}
	s.mutex.Unlock()
	
	s.writeJSON(w, map[string]interface{}{
		"kind":       "Scale",
		"apiVersion": "autoscaling/v1",
		"spec": map[string]int32{
			"replicas": scaleReq.Spec.Replicas,
		},
	})
}

// StatefulSet handlers (similar to Deployment)

func (s *Server) listStatefulSetsHandler(w http.ResponseWriter, r *http.Request) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	var statefulsets []*api.StatefulSet
	for _, ss := range s.statefulsets {
		statefulsets = append(statefulsets, ss)
	}
	
	s.writeJSON(w, map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSetList",
		"items":      statefulsets,
	})
}

func (s *Server) createStatefulSetHandler(w http.ResponseWriter, r *http.Request) {
	var statefulset api.StatefulSet
	if err := s.decodeManifest(r, &statefulset); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	s.setDefaultsForStatefulSet(&statefulset)
	
	s.mutex.Lock()
	s.statefulsets[statefulset.Name] = &statefulset
	s.mutex.Unlock()
	
	if err := s.storage.StoreStatefulSet(&statefulset); err != nil {
		log.Printf("Failed to persist statefulset: %v", err)
	}
	
	w.WriteHeader(http.StatusCreated)
	s.writeJSON(w, &statefulset)
}

func (s *Server) getStatefulSetHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.RLock()
	ss, exists := s.statefulsets[name]
	s.mutex.RUnlock()
	
	if !exists {
		s.writeError(w, http.StatusNotFound, "StatefulSet not found", nil)
		return
	}
	
	s.writeJSON(w, ss)
}

func (s *Server) updateStatefulSetHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	var statefulset api.StatefulSet
	if err := s.decodeManifest(r, &statefulset); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	statefulset.Name = name
	s.setDefaultsForStatefulSet(&statefulset)
	
	s.mutex.Lock()
	s.statefulsets[name] = &statefulset
	s.mutex.Unlock()
	
	if err := s.storage.StoreStatefulSet(&statefulset); err != nil {
		log.Printf("Failed to persist statefulset: %v", err)
	}
	
	s.writeJSON(w, &statefulset)
}

func (s *Server) deleteStatefulSetHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.Lock()
	delete(s.statefulsets, name)
	s.mutex.Unlock()
	
	if err := s.storage.DeleteStatefulSet(name); err != nil {
		log.Printf("Failed to delete statefulset from storage: %v", err)
	}
	
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) scaleStatefulSetHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	var scaleReq struct {
		Spec struct {
			Replicas int32 `json:"replicas"`
		} `json:"spec"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&scaleReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid scale request", err)
		return
	}
	
	s.mutex.Lock()
	if ss, exists := s.statefulsets[name]; exists {
		ss.Spec.Replicas = &scaleReq.Spec.Replicas
	}
	s.mutex.Unlock()
	
	s.writeJSON(w, map[string]interface{}{
		"kind":       "Scale",
		"apiVersion": "autoscaling/v1",
		"spec": map[string]int32{
			"replicas": scaleReq.Spec.Replicas,
		},
	})
}

// Service handlers (updated to use Kubernetes Service type)

func (s *Server) listServicesHandler(w http.ResponseWriter, r *http.Request) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	var services []*api.Service
	for _, service := range s.services {
		services = append(services, service)
	}
	
	s.writeJSON(w, map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceList",
		"items":      services,
	})
}

func (s *Server) createServiceHandler(w http.ResponseWriter, r *http.Request) {
	var service api.Service
	if err := s.decodeManifest(r, &service); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	s.setDefaultsForService(&service)
	
	s.mutex.Lock()
	s.services[service.Name] = &service
	s.mutex.Unlock()
	
	if err := s.storage.StoreService(&service); err != nil {
		log.Printf("Failed to persist service: %v", err)
	}
	
	w.WriteHeader(http.StatusCreated)
	s.writeJSON(w, &service)
}

func (s *Server) getServiceHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.RLock()
	service, exists := s.services[name]
	s.mutex.RUnlock()
	
	if !exists {
		s.writeError(w, http.StatusNotFound, "Service not found", nil)
		return
	}
	
	s.writeJSON(w, service)
}

func (s *Server) updateServiceHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	var service api.Service
	if err := s.decodeManifest(r, &service); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	service.Name = name
	s.setDefaultsForService(&service)
	
	s.mutex.Lock()
	s.services[name] = &service
	s.mutex.Unlock()
	
	if err := s.storage.StoreService(&service); err != nil {
		log.Printf("Failed to persist service: %v", err)
	}
	
	s.writeJSON(w, &service)
}

func (s *Server) deleteServiceHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.Lock()
	delete(s.services, name)
	s.mutex.Unlock()
	
	if err := s.storage.DeleteService(name); err != nil {
		log.Printf("Failed to delete service from storage: %v", err)
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// Node handlers (keep existing implementation)

func (s *Server) listNodesHandler(w http.ResponseWriter, r *http.Request) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	var nodes []*api.Node
	for _, node := range s.nodes {
		nodes = append(nodes, node)
	}
	
	s.writeJSON(w, map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "NodeList",
		"items":      nodes,
	})
}

func (s *Server) getNodeHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.RLock()
	node, exists := s.nodes[name]
	s.mutex.RUnlock()
	
	if !exists {
		s.writeError(w, http.StatusNotFound, "Node not found", nil)
		return
	}
	
	s.writeJSON(w, node)
}

// Generic workload list for backward compatibility
func (s *Server) listAllWorkloadsHandler(w http.ResponseWriter, r *http.Request) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	var workloads []interface{}
	
	for _, pod := range s.pods {
		workloads = append(workloads, pod)
	}
	for _, deployment := range s.deployments {
		workloads = append(workloads, deployment)
	}
	for _, ss := range s.statefulsets {
		workloads = append(workloads, ss)
	}
	
	s.writeJSON(w, map[string]interface{}{
		"items": workloads,
		"count": len(workloads),
	})
}

// Handle getting a specific workload by name
func (s *Server) getGenericWorkloadHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	// Check deployments first
	if deployment, exists := s.deployments[name]; exists {
		s.writeJSON(w, deployment)
		return
	}
	
	// Check statefulsets
	if statefulset, exists := s.statefulsets[name]; exists {
		s.writeJSON(w, statefulset)
		return
	}
	
	// Check pods
	if pod, exists := s.pods[name]; exists {
		s.writeJSON(w, pod)
		return
	}
	
	// Workload not found
	s.writeError(w, http.StatusNotFound, "Workload not found", nil)
}

// Handle updating a specific workload
func (s *Server) updateGenericWorkloadHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	// Decode the manifest to determine its kind
	var manifest map[string]interface{}
	if err := s.decodeManifest(r, &manifest); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	kind, ok := manifest["kind"].(string)
	if !ok {
		s.writeError(w, http.StatusBadRequest, "Missing or invalid 'kind' field", nil)
		return
	}
	
	// Re-encode the manifest for type-specific handlers
	data, _ := json.Marshal(manifest)
	
	// Route to the appropriate handler based on kind
	switch kind {
	case "Deployment":
		var deployment api.Deployment
		if err := json.Unmarshal(data, &deployment); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid Deployment manifest", err)
			return
		}
		deployment.Name = name
		s.setDefaultsForDeployment(&deployment)
		s.mutex.Lock()
		s.deployments[name] = &deployment
		s.mutex.Unlock()
		if err := s.storage.StoreDeployment(&deployment); err != nil {
			log.Printf("Failed to persist deployment: %v", err)
		}
		s.writeJSON(w, &deployment)
		
	case "StatefulSet":
		var statefulset api.StatefulSet
		if err := json.Unmarshal(data, &statefulset); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid StatefulSet manifest", err)
			return
		}
		statefulset.Name = name
		s.setDefaultsForStatefulSet(&statefulset)
		s.mutex.Lock()
		s.statefulsets[name] = &statefulset
		s.mutex.Unlock()
		if err := s.storage.StoreStatefulSet(&statefulset); err != nil {
			log.Printf("Failed to persist statefulset: %v", err)
		}
		s.writeJSON(w, &statefulset)
		
	case "Pod":
		var pod api.Pod
		if err := json.Unmarshal(data, &pod); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid Pod manifest", err)
			return
		}
		pod.Name = name
		s.setDefaultsForPod(&pod)
		s.mutex.Lock()
		s.pods[name] = &pod
		s.mutex.Unlock()
		if err := s.storage.StorePod(&pod); err != nil {
			log.Printf("Failed to persist pod: %v", err)
		}
		s.writeJSON(w, &pod)
		
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported workload kind: %s", kind), nil)
	}
}

// Handle deleting a specific workload
func (s *Server) deleteGenericWorkloadHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	var deleted bool
	
	// Try to delete from deployments
	if _, exists := s.deployments[name]; exists {
		delete(s.deployments, name)
		if err := s.storage.DeleteDeployment(name); err != nil {
			log.Printf("Failed to delete deployment from storage: %v", err)
		}
		deleted = true
	}
	
	// Try to delete from statefulsets
	if _, exists := s.statefulsets[name]; exists {
		delete(s.statefulsets, name)
		if err := s.storage.DeleteStatefulSet(name); err != nil {
			log.Printf("Failed to delete statefulset from storage: %v", err)
		}
		deleted = true
	}
	
	// Try to delete from pods
	if _, exists := s.pods[name]; exists {
		delete(s.pods, name)
		if err := s.storage.DeletePod(name); err != nil {
			log.Printf("Failed to delete pod from storage: %v", err)
		}
		deleted = true
	}
	
	if deleted {
		w.WriteHeader(http.StatusNoContent)
	} else {
		s.writeError(w, http.StatusNotFound, "Workload not found", nil)
	}
}

// Handle creating a new workload
func (s *Server) createGenericWorkloadHandler(w http.ResponseWriter, r *http.Request) {
	// Decode the manifest to determine its kind
	var manifest map[string]interface{}
	if err := s.decodeManifest(r, &manifest); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	kind, ok := manifest["kind"].(string)
	if !ok {
		s.writeError(w, http.StatusBadRequest, "Missing or invalid 'kind' field", nil)
		return
	}
	
	// Re-encode the manifest for type-specific handlers
	data, _ := json.Marshal(manifest)
	
	// Route to the appropriate handler based on kind
	switch kind {
	case "Deployment":
		var deployment api.Deployment
		if err := json.Unmarshal(data, &deployment); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid Deployment manifest", err)
			return
		}
		s.setDefaultsForDeployment(&deployment)
		s.mutex.Lock()
		s.deployments[deployment.Name] = &deployment
		s.mutex.Unlock()
		if err := s.storage.StoreDeployment(&deployment); err != nil {
			log.Printf("Failed to persist deployment: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		s.writeJSON(w, &deployment)
		
	case "StatefulSet":
		var statefulset api.StatefulSet
		if err := json.Unmarshal(data, &statefulset); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid StatefulSet manifest", err)
			return
		}
		s.setDefaultsForStatefulSet(&statefulset)
		s.mutex.Lock()
		s.statefulsets[statefulset.Name] = &statefulset
		s.mutex.Unlock()
		if err := s.storage.StoreStatefulSet(&statefulset); err != nil {
			log.Printf("Failed to persist statefulset: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		s.writeJSON(w, &statefulset)
		
	case "Pod":
		var pod api.Pod
		if err := json.Unmarshal(data, &pod); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid Pod manifest", err)
			return
		}
		s.setDefaultsForPod(&pod)
		s.mutex.Lock()
		s.pods[pod.Name] = &pod
		s.mutex.Unlock()
		if err := s.storage.StorePod(&pod); err != nil {
			log.Printf("Failed to persist pod: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		s.writeJSON(w, &pod)
		
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported workload kind: %s", kind), nil)
	}
}

// Container and system handlers (keep existing implementation)
func (s *Server) listContainersHandler(w http.ResponseWriter, r *http.Request) {
	containers, err := s.runtime.ListContainers(r.Context(), runtime.ContainerFilter{
		Labels: map[string]string{
			"managed-by": "synthesis",
		},
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list containers", err)
		return
	}
	
	s.writeJSON(w, map[string]interface{}{
		"items": containers,
		"count": len(containers),
	})
}

func (s *Server) getContainerLogsHandler(w http.ResponseWriter, r *http.Request) {
	containerID := mux.Vars(r)["id"]
	
	logs, err := s.runtime.GetContainerLogs(r.Context(), containerID, runtime.LogOptions{
		Stdout: true,
		Stderr: true,
		Tail:   "100",
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to get container logs", err)
		return
	}
	defer logs.Close()
	
	w.Header().Set("Content-Type", "text/plain")
	if _, err := w.Write([]byte("Logs for container " + containerID + ":\n")); err != nil {
		log.Printf("Failed to write logs: %v", err)
	}
}

func (s *Server) execContainerHandler(w http.ResponseWriter, r *http.Request) {
	containerID := mux.Vars(r)["id"]
	
	var execReq struct {
		Command []string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&execReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}
	
	result, err := s.runtime.ExecContainer(r.Context(), containerID, execReq.Command)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to execute command", err)
		return
	}
	
	s.writeJSON(w, result)
}

func (s *Server) systemInfoHandler(w http.ResponseWriter, r *http.Request) {
	info, err := s.runtime.GetSystemInfo(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to get system info", err)
		return
	}
	
	s.writeJSON(w, info)
}

// Apply manifest handler (auto-detect resource type)
func (s *Server) applyManifestHandler(w http.ResponseWriter, r *http.Request) {
	var manifest map[string]interface{}
	if err := s.decodeManifest(r, &manifest); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid manifest", err)
		return
	}
	
	kind, ok := manifest["kind"].(string)
	if !ok {
		s.writeError(w, http.StatusBadRequest, "Missing or invalid 'kind' field", nil)
		return
	}
	
	// Re-decode based on kind
	data, _ := json.Marshal(manifest)
	
	switch kind {
	case "Pod":
		var pod api.Pod
		if err := json.Unmarshal(data, &pod); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid Pod manifest", err)
			return
		}
		s.setDefaultsForPod(&pod)
		s.mutex.Lock()
		s.pods[pod.Name] = &pod
		s.mutex.Unlock()
		s.storage.StorePod(&pod)
		s.writeJSON(w, &pod)
		
	case "Deployment":
		var deployment api.Deployment
		if err := json.Unmarshal(data, &deployment); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid Deployment manifest", err)
			return
		}
		s.setDefaultsForDeployment(&deployment)
		s.mutex.Lock()
		s.deployments[deployment.Name] = &deployment
		s.mutex.Unlock()
		s.storage.StoreDeployment(&deployment)
		s.writeJSON(w, &deployment)
		
	case "StatefulSet":
		var statefulset api.StatefulSet
		if err := json.Unmarshal(data, &statefulset); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid StatefulSet manifest", err)
			return
		}
		s.setDefaultsForStatefulSet(&statefulset)
		s.mutex.Lock()
		s.statefulsets[statefulset.Name] = &statefulset
		s.mutex.Unlock()
		s.storage.StoreStatefulSet(&statefulset)
		s.writeJSON(w, &statefulset)
		
	case "Service":
		var service api.Service
		if err := json.Unmarshal(data, &service); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid Service manifest", err)
			return
		}
		s.setDefaultsForService(&service)
		s.mutex.Lock()
		s.services[service.Name] = &service
		s.mutex.Unlock()
		s.storage.StoreService(&service)
		s.writeJSON(w, &service)
		
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported resource kind: %s", kind), nil)
	}
}

// Helper methods

func (s *Server) decodeManifest(r *http.Request, v interface{}) error {
	contentType := r.Header.Get("Content-Type")
	
	if strings.Contains(contentType, "application/yaml") || strings.HasSuffix(r.URL.Path, ".yaml") {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		return yaml.Unmarshal(data, v)
	}
	
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) setDefaultsForPod(pod *api.Pod) {
	if pod.APIVersion == "" {
		pod.APIVersion = "v1"
	}
	if pod.Kind == "" {
		pod.Kind = "Pod"
	}
	if pod.CreationTimestamp.IsZero() {
		pod.CreationTimestamp = metav1.NewTime(time.Now())
	}
}

func (s *Server) setDefaultsForDeployment(deployment *api.Deployment) {
	if deployment.APIVersion == "" {
		deployment.APIVersion = "apps/v1"
	}
	if deployment.Kind == "" {
		deployment.Kind = "Deployment"
	}
	if deployment.CreationTimestamp.IsZero() {
		deployment.CreationTimestamp = metav1.NewTime(time.Now())
	}
	if deployment.Spec.Replicas == nil {
		replicas := int32(1)
		deployment.Spec.Replicas = &replicas
	}
}

func (s *Server) setDefaultsForStatefulSet(ss *api.StatefulSet) {
	if ss.APIVersion == "" {
		ss.APIVersion = "apps/v1"
	}
	if ss.Kind == "" {
		ss.Kind = "StatefulSet"
	}
	if ss.CreationTimestamp.IsZero() {
		ss.CreationTimestamp = metav1.NewTime(time.Now())
	}
	if ss.Spec.Replicas == nil {
		replicas := int32(1)
		ss.Spec.Replicas = &replicas
	}
}

func (s *Server) setDefaultsForService(service *api.Service) {
	if service.APIVersion == "" {
		service.APIVersion = "v1"
	}
	if service.Kind == "" {
		service.Kind = "Service"
	}
	if service.CreationTimestamp.IsZero() {
		service.CreationTimestamp = metav1.NewTime(time.Now())
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	
	errorResp := map[string]interface{}{
		"error":     message,
		"timestamp": time.Now().Unix(),
	}
	
	if err != nil && s.config.Debug {
		errorResp["details"] = err.Error()
	}
	
	json.NewEncoder(w).Encode(errorResp)
}

func (s *Server) loadState() error {
	// Load pods
	pods, err := s.storage.ListPods()
	if err != nil {
		return fmt.Errorf("failed to load pods: %w", err)
	}
	for _, pod := range pods {
		s.pods[pod.Name] = pod
	}
	
	// Load deployments
	deployments, err := s.storage.ListDeployments()
	if err != nil {
		return fmt.Errorf("failed to load deployments: %w", err)
	}
	for _, deployment := range deployments {
		s.deployments[deployment.Name] = deployment
	}
	
	// Load statefulsets
	statefulsets, err := s.storage.ListStatefulSets()
	if err != nil {
		return fmt.Errorf("failed to load statefulsets: %w", err)
	}
	for _, ss := range statefulsets {
		s.statefulsets[ss.Name] = ss
	}
	
	// Load services
	services, err := s.storage.ListServices()
	if err != nil {
		return fmt.Errorf("failed to load services: %w", err)
	}
	for _, service := range services {
		s.services[service.Name] = service
	}
	
	log.Printf("Loaded %d pods, %d deployments, %d statefulsets, %d services from storage", 
		len(pods), len(deployments), len(statefulsets), len(services))
	return nil
}

func (s *Server) runNodeController(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateNodeStatus(ctx)
		}
	}
}

func (s *Server) updateNodeStatus(ctx context.Context) {
	info, err := s.runtime.GetSystemInfo(ctx)
	if err != nil {
		log.Printf("Failed to get system info for node update: %v", err)
		return
	}
	
	node := &api.Node{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Node",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-node",
		},
		Status: api.NodeStatus{
			Capacity: api.ResourceList{
				api.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", info.NCPU)),
				api.ResourceMemory: resource.MustParse(fmt.Sprintf("%d", info.MemTotal)),
			},
			Allocatable: api.ResourceList{
				api.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", info.NCPU)),
				api.ResourceMemory: resource.MustParse(fmt.Sprintf("%d", info.MemTotal)),
			},
			Conditions: []api.NodeCondition{
				{
					Type:               api.NodeReady,
					Status:             api.ConditionTrue,
					LastHeartbeatTime:  metav1.NewTime(time.Now()),
					LastTransitionTime: metav1.NewTime(time.Now()),
					Reason:             "NodeReady",
					Message:            "Node is ready",
				},
			},
			NodeInfo: api.NodeSystemInfo{
				MachineID:               "local",
				SystemUUID:              "local",
				BootID:                  "local",
				KernelVersion:           info.KernelVersion,
				OSImage:                 info.OperatingSystem,
				ContainerRuntimeVersion: info.RuntimeVersion,
				Architecture:            info.Architecture,
				OperatingSystem:         info.OperatingSystem,
			},
		},
	}
	
	s.mutex.Lock()
	s.nodes["local-node"] = node
	s.mutex.Unlock()
} 