import docker
import schedule
import time
import threading
import redis
import subprocess

class Haven:
    def __init__(self, name, capacity):
        self.name = name
        self.capacity = capacity
        self.client = docker.from_env()
        self.redis_connection = redis.StrictRedis(host='localhost', port=6379, db=0)
        schedule.every(15).seconds.do(self.allocate_resources_job)
        schedule.every(1).seconds.do(self.check_container_health)

        self.schedule_thread = threading.Thread(target=self.schedule_loop, daemon=True)
        self.schedule_thread.start()

    def run_container(self, container_name, image):
        try:
            command = f"ip netns exec {self.name} docker run --name {container_name} -d {image}"
            subprocess.run(command, shell=True, check=True)

            # Store container information in Redis
            container = self.client.containers.get(container_name)
            self.redis_connection.set(container_name, container.id)
            print(f"Started container '{container_name}'")

        except Exception as e:
            print(f"Failed to start container '{container_name}': {str(e)}")

    def list_containers(self):
        containers = self.client.containers.list()
        for container in containers:
            print(f"Container ID: {container.id}, Name: {container.name}, Status: {container.status}")
    
    def check_container_health(self):
        container_names = self.redis_connection.keys()
        for container_name in container_names:
            try:
                container_id = self.redis_connection.get(container_name).decode()
                container = self.client.containers.get(container_id)
                container_status = container.status

                if container_status in {"dead", "removing", "exited", "paused"}:
                    print(f"Container '{container_name}' is in status '{container_status}'. Restarting...")

                    container.remove(force=True)
                    self.redis_connection.delete(container_name)

                    self.run_container(container_name.decode(), container.attrs["Config"]["Image"])

            except docker.errors.NotFound:
                print(f"Container '{container_name}' not found. It might have been removed.")
                self.redis_connection.delete(container_name)

            except Exception as e:
                print(f"Failed to check container health for '{container_name}': {str(e)}")

    def allocate_resources_job(self):
        try:
            container_names = self.redis_connection.keys()
            for container_name in container_names:
                container_id = self.redis_connection.get(container_name).decode()
                container = self.client.containers.get(container_id)
                container_stats = container.stats(stream=False)
                container_name = container.name
                cpu_utilization = container_stats["cpu_stats"]["cpu_usage"]["total_usage"]
                memory_usage = container_stats["memory_stats"]["usage"]
                new_cpu_limit = int(self.capacity.cpu_cores * (cpu_utilization / 1000000000))
                container.update(cpu_period=100000, cpu_quota=new_cpu_limit)
                new_memory_limit = max(int(self.capacity.memory_mb * (memory_usage / (1024 * 1024))), 6)
                container.update(mem_limit=new_memory_limit)
                print(f"Adjusted resources for '{container_name}': CPU {new_cpu_limit}, Memory {new_memory_limit}MB")

        except Exception as e:
            print(f"Failed to allocate resources: {str(e)}")

    def schedule_loop(self):
        while True:
            schedule.run_pending()
            time.sleep(1)

    def delete_container(self, container_name):
        try:
            container_id = self.redis_connection.get(container_name).decode()
            container = self.client.containers.get(container_id)

            if container:
                container.stop()
                container.remove()
                self.redis_connection.delete(container_name)
                print(f"Deleted container '{container_name}'")
            else:
                print(f"Container '{container_name}' not found")
        except Exception as e:
            print(f"Failed to delete container '{container_name}': {str(e)}")

class Capacity:
    def __init__(self, cpu_cores, memory_mb, storage_gb):
        self.cpu_cores = cpu_cores
        self.memory_mb = memory_mb
        self.storage_gb = storage_gb

