
import requests
import subprocess
import time

# FIXME


class Ingress:

    def __init__(self, name):
        self.name = name
        self.admin_url = None

    # FIXME
    def start_ingress(self, network_name):
        try:
            subprocess.run(["docker", "run", "--name",
                           f"ingress-{network_name}", "--network", network_name, "-d", "kong:latest"])

            # FIXME
            time.sleep(5)
            inspect_output = subprocess.check_output(
                ["docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", f"ingress-{network_name}"])
            container_ip = inspect_output.decode().strip()

            self.admin_url = f"http://{container_ip}:8001"
            print(f"Kong Admin URL for '{self.name}': {self.admin_url}")

        except Exception as e:
            print(f"Failed to start Kong APG '{self.name}': {str(e)}")

    def register_ingress_service(self, name, url):
        try:
            # Define the Kong service registration endpoint
            registration_endpoint = f"{self.admin_url}/services"

            # Kong Service create payload
            payload = {
                "name": name,
                "url": url,
            }

            response = requests.post(registration_endpoint, json=payload)

            if response.status_code == 201:
                print(f"Registered Ingress service '{name}'")
            else:
                print(
                    f"Failed to register Ingress service '{name}' with status code {response.status_code}")

        except Exception as e:
            print(f"Failed to register Ingress service '{name}': {str(e)}")

    def register_ingress_route(self, name, service_name):
        try:
            route_creation_endpoint = f"{self.admin_url}/services/{service_name}/routes"

            # Kong route create payload
            payload = {
                "name": name,
                "paths": [f"/{service_name}"],
            }

            response = requests.post(route_creation_endpoint, json=payload)

            if response.status_code == 201:
                print(
                    f"Created Ingress route '{name}' for service '{service_name}'")
            else:
                print(
                    f"Failed to create Ingress route '{name}' for service '{service_name}' with status code {response.status_code}")

        except Exception as e:
            print(
                f"Failed to create Ingress route '{name}' for service '{service_name}': {str(e)}")
