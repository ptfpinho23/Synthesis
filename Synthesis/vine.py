from haven import Haven, Capacity
from vine_networking import VineNetworking
import time


class Vine:
    
    def __init__(self):
        self.havens: dict[str, Haven] = {}
        self.networking = VineNetworking()

    def create_haven(self, name, capacity):
        haven = Haven(name, capacity)
        self.networking.create_network(name)
        self.havens[name] = haven

    def delete_haven(self, name):
        if name in self.havens:
            del self.havens[name]


    def attach_container_to_haven(self, haven_name, container_name, container_image):
        if haven_name not in self.havens:
            return
        print(self.havens[haven_name])
        container = self.havens[haven_name].run_container(container_name, container_image)
        self.networking.connect_container_to_network(container, haven_name)
    

    def list_containers(self):
        for haven in self.havens.values():
            haven.list_containers()

    def cleanup(self):
        for haven in self.havens.values():
            haven.cleanup()
        vine_networking = VineNetworking()
        for haven_name in vine_networking.docker_client.networks.list(names=self.havens.keys()):
            vine_networking.remove_network(haven_name)

def main():
    capacity = Capacity(cpu_cores=4, memory_mb=8192, storage_gb=100)
    vine = Vine()
    vine.create_haven("haven1", capacity)
    vine.attach_container_to_haven("haven1", "ubuntu_test_1", "ubuntu2")
    vine.attach_container_to_haven("haven1", "ubuntu-test_2", "ubuntu2  ")

    vine.list_containers()

    while True:
        time.sleep(1)

if __name__ == "__main__":
    main()
