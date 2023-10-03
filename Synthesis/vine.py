from haven import Haven, Capacity
from vine_networking import VineNetworking
import time


class Vine:
    
    def __init__(self):
        self.havens: dict[str, Haven] = {}

    def create_haven(self, name, capacity):
        haven = Haven(name, capacity)
        VineNetworking().create_network(name, 'vine_bridge', 'veth1', 'veth2', '10.0.0.3', '24', 'eth0')
        self.havens[name] = haven

    def delete_haven(self, name):
        if name in self.havens:
            del self.havens[name]


    def attach_container_to_haven(self, haven_key, container_name, container_image):
        if haven_key not in self.havens:
            return
        print(self.havens[haven_key])
        self.havens[haven_key].run_container(container_name, container_image)
    

    def list_containers(self):
        for haven in self.havens.values():
            haven.list_containers()

        
def main():
    capacity = Capacity(cpu_cores=4, memory_mb=8192, storage_gb=100)
    vine = Vine()
    vine.create_haven("haven1", capacity)
    vine.attach_container_to_haven("haven1", "nginx_test1", "nginx:latest")
    vine.attach_container_to_haven("haven1", "nginx_test2", "nginx:latest")

    vine.list_containers()

    while True:
        time.sleep(1)

if __name__ == "__main__":
    main()
