
import requests
import subprocess
import time

# FIXME


class Ingress:

    def __init__(self):
        self.admin_url = None

    # FIXME

    def start_ingress(self, network_name):
        try:
            subprocess.run(["docker", "run", "--name",
                            f"kong-database-{network_name}", "--network", network_name, "-d", "-p", "5432:5432", "-e", "POSTGRES_USER=kong", "-e", "POSTGRES_DB=kong", "-e", "POSTGRES_PASSWORD=kong", "postgres:latest"])

            time.sleep(15)

            # run Kong bootstrap
            subprocess.run([
                "docker", "run",
                "--network", network_name, "-d", "-e", "KONG_DATABASE=postgres",
                "-e", f"KONG_DATABASE=postgres",
                "-e", f"KONG_PG_HOST=kong-database-{network_name}",
                "-e", f"KONG_PG_USER=kong",
                "-e", f"KONG_PG_PASSWORD=kong",
                "kong:latest",
                "kong", "migrations", "bootstrap"
            ])

            # start Kong
            subprocess.run([
                "docker", "run", "--rm", "--network", network_name,
                "--name", f"ingress-{network_name}",
                "-d",
                "-e", "KONG_DATABASE=postgres",
                "-e", f"KONG_PG_HOST=kong-database-{network_name}",
                "-e", "KONG_PG_USER=kong",
                "-e", "KONG_PG_PASSWORD=kong",
                "-e", "KONG_ADMIN_LISTEN=0.0.0.0:8001, 0.0.0.0:8444 ssl",
                "-p", "8000:8000",
                "-p", "8443:8443",
                "-p", "8444:8444",
                "-p", "8001:8001",
                f"kong:latest"
            ])

            # Get the container IP and set the admin URL
            inspect_output = subprocess.check_output([
                "docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
                f"ingress-{network_name}"
            ])
            
            container_ip = inspect_output.decode().strip()

            self.admin_url = f"http://localhost:8001"
            print(f"Kong Admin URL for '{network_name}': {self.admin_url}")

            time.sleep(2)

        except Exception as e:
            print(f"Failed to start Kong APG '{network_name}': {str(e)}")

    def register_ingress_service(self, name, url):
        try:
            kong_admin_url = "http://localhost:8001"

            registration_endpoint = f"{kong_admin_url}/services"

            payload = {
            "name": name,
            "url": url,
            "protocol": "http",
            "port": 80,
            "path": "/",
        }

            print("printing registration endpoint")
            print(registration_endpoint)
            response = requests.post(registration_endpoint, json=payload)

            if response.status_code == 201:
                print(f"Registered Ingress service '{name}'")
            else:
                print(
                    f"Failed to register Ingress service '{name}' with status code {response.status_code}")

        except Exception as e:
            print(f"Failed to register Ingress service '{name}': {str(e)}")

    def register_ingress_route(self, service_name, route_path):
        try:
            route_creation_endpoint = f"{self.admin_url}/services/{service_name}/routes"

            # Kong route create payload
            payload = {
                "name": service_name,
                "protocols": ["http"],
                "methods": ["GET", "POST"],
                "paths": ["/"],
            }

            response = requests.post(route_creation_endpoint, json=payload)

            print("printing ingress route posting response")
            print(response.content)
            if response.status_code == 201:
                print(
                    f"Created Ingress route '{service_name}' for service '{service_name}'")
            else:
                print(
                    f"Failed to create Ingress route '{service_name}' for service '{service_name}' with status code {response.status_code}")

        except Exception as e:
            print(
                f"Failed to create Ingress route '{service_name}' for service '{service_name}': {str(e)}")
