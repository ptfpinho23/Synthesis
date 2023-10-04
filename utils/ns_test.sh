container1_ip=$(ip netns exec haven1 docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ubuntu_1)
container2_ip=$(ip netns exec haven1 docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ubuntu_2)

echo "Container 1 IP: $container1_ip"
echo "Container 2 IP: $container2_ip"
