import docker
import subprocess

class VineNetworking:
    def __init__(self):
        self.docker_client = docker.from_env()

    def create_network(self, network_name):
        try:
            existing_network = self.docker_client.networks.get(network_name)
            print(f"Using existing Docker network: {network_name}")
            return existing_network
        except docker.errors.NotFound:
            try:
                new_network = self.docker_client.networks.create(
                    network_name,
                    driver="bridge",
                )
                print(f"Created Docker network: {network_name}")
                return new_network
            except docker.errors.APIError as e:
                print(f"Failed to create network: {str(e)}")
                return None

    def remove_network(self, network_name):
        try:
            network = self.docker_client.networks.get(network_name)
            network.remove()
            print(f"Removed Docker network: {network_name}")
        except docker.errors.NotFound:
            print(f"Network not found: {network_name}")
        except docker.errors.APIError as e:
            print(f"Failed to remove network: {str(e)}")

    def connect_container_to_network(self, container, network_name):
        try:
            network = self.docker_client.networks.get(network_name)
            network.connect(container)
            print(f"Connected container {container} to network {network_name}")
        except docker.errors.NotFound:
            print(f"Network {network_name} not found.")
        except docker.errors.APIError as e:
            print(f"Failed to connect container {container} to network {network_name}: {str(e)}")

    def disconnect_container_from_network(self, container_name, network_name):
        try:
            container = self.docker_client.containers.get(container_name)
            container.disconnect(network_name)
            print(f"Disconnected container '{container_name}' from network '{network_name}'")
        except docker.errors.NotFound:
            print(f"Container not found: {container_name}")
        except docker.errors.APIError as e:
            print(f"Failed to disconnect container from network: {str(e)}")
