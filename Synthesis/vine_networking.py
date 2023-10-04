import subprocess

class VineNetworking:
    def __init__(self):
        self.namespaces = set()

    def create_namespace(self, name):
        subprocess.run(["ip", "netns", "add", name])
        self.namespaces.add(name)

    def delete_namespace(self, name):
        subprocess.run(["ip", "netns", "delete", name])
        self.namespaces.remove(name)

    def create_bridge(self, bridge_name):
        subprocess.run(["ip", "link", "add", bridge_name, "type", "bridge"])
        subprocess.run(["ip", "link", "set", bridge_name, "up"])

    def create_veth_pair(self, namespace, veth_name, peer_name):
        subprocess.run(["ip", "link", "add", veth_name, "type", "veth", "peer", "name", peer_name, "netns", namespace])

    def assign_ip(self, interface, ip_address, subnet_mask):
        subprocess.run(["ip", "addr", "add", f"{ip_address}/{subnet_mask}", "dev", interface])
        subprocess.run(["ip", "link", "set", interface, "up"])

    def enable_ip_forwarding(self):
        with open("/proc/sys/net/ipv4/ip_forward", "w") as ipf:
            ipf.write("1")

    def configure_nat(self, external_interface):
        subprocess.run(["iptables", "-t", "nat", "-A", "POSTROUTING", "-o", external_interface, "-j", "MASQUERADE"])

    def configure_routing(self, namespace, gateway_ip):
        subprocess.run(["ip", "netns", "exec", namespace, "ip", "route", "add", "default", "via", gateway_ip])

    def remove_bridge(self, bridge_name):
        subprocess.run(["ip", "link", "del", bridge_name])

    def remove_veth_pair(self, veth_name):
        subprocess.run(["ip", "link", "del", veth_name])

    def create_network(self, namespace_name, bridge_name, veth_name, peer_name, ip_address, subnet_mask, external_interface):
        self.create_namespace(namespace_name)
        self.create_bridge(bridge_name)
        self.create_veth_pair(namespace_name, veth_name, peer_name)
        self.assign_ip(veth_name, ip_address, subnet_mask)
        self.enable_ip_forwarding()
        self.configure_nat(external_interface)
        self.configure_routing(namespace_name, "10.0.0.1")
        namespace_gateway_ip = f"{ip_address[:-1]}1"
        self.configure_routing(namespace_name, namespace_gateway_ip)

    def cleanup(self):
        for namespace in self.namespaces:
            self.delete_namespace(namespace)

    def configure_routing(self, namespace, gateway_ip):
        # Add a default route within the namespace
        subprocess.run(["ip", "netns", "exec", namespace, "ip", "route", "add", "default", "via", gateway_ip])
