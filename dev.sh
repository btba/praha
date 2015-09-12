#!/bin/bash
#
# Builds images & runs on OS X, mounting local directories.

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <bookings_dsn>"
    exit 1
fi

set -eux

docker build -t mygorez gorez

docker rm -f devnginx devgorez 2> /dev/null || true
docker rmi $(docker images -f "dangling=true" -q) 2> /dev/null || true

docker run -d -P -e "BOOKINGS_DSN=$1" -v $(pwd)/gorez/templates:/gorez/templates --name devgorez mygorez
docker run -d -P -v $(pwd)/nginx/content:/usr/share/nginx/html -v $(pwd)/nginx/conf/nginx.conf:/etc/nginx/nginx.conf --link devgorez:gorez --name devnginx nginx

echo http://$(docker-machine ip default):$(docker inspect --format='{{index .NetworkSettings.Ports "80/tcp" 0 "HostPort"}}' devnginx)
