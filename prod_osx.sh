#!/bin/bash
#
# Runs production images on OS X.

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <env_file>"
    exit 1
fi

set -eux

docker rm -f prodnginx prodfiles prodgorez 2> /dev/null || true

docker pull btba/praha-gorez
docker pull btba/praha-nginx

docker run -d -P --env-file "$1" --name prodgorez btba/praha-gorez
docker run -d --name prodfiles btba/praha-nginx
docker run -d -P --volumes-from prodfiles --link prodgorez:gorez --name prodnginx nginx

echo http://$(docker-machine ip default):$(docker inspect --format='{{index .NetworkSettings.Ports "80/tcp" 0 "HostPort"}}' prodnginx)
