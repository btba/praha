#!/bin/bash
#
# Runs production stack.

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <bookings_dsn>"
    exit 1
fi

set -eux

docker rm -f prodnginx prodfiles prodgorez 2> /dev/null || true

docker run -d -P -e "BOOKINGS_DSN=$1" --name prodgorez btba/praha-gorez
docker run -d --name prodfiles btba/praha-nginx
docker run -d -P --volumes-from prodfiles --link prodgorez:gorez --name prodnginx nginx

echo http://$(boot2docker ip):$(docker inspect --format='{{index .NetworkSettings.Ports "80/tcp" 0 "HostPort"}}' prodnginx)
