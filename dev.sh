#!/bin/bash
#
# Builds & runs development stack on OS X.

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <bookings_dsn>"
    exit 1
fi

set -eux

docker build -t mygorez gorez
docker build -t myfiles nginx

docker rm -f devnginx devfiles devgorez 2> /dev/null || true
docker rmi $(docker images -f "dangling=true" -q) 2> /dev/null || true

docker run -d -P -e "BOOKINGS_DSN=$1" --name devgorez mygorez
docker run -d --name devfiles myfiles
docker run -d -P --volumes-from devfiles --link devgorez:gorez --name devnginx nginx

echo http://$(boot2docker ip):$(docker inspect --format='{{index .NetworkSettings.Ports "80/tcp" 0 "HostPort"}}' devnginx)
