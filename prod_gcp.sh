#!/bin/bash
#
# Runs production images on GCP.

if [ "$#" -ne 3 ]; then
    echo "Usage: $0 <bookings_dsn> <stripe_secret_key> <stripe_publishable_key>"
    exit 1
fi

set -eux

docker rm -f prodnginx prodfiles prodgorez 2> /dev/null || true

docker pull btba/praha-gorez
docker pull btba/praha-nginx

docker run -d -P  -e "BOOKINGS_DSN=$1" -e "STRIPE_SECRET=$2" -e "STRIPE_PUBLISHABLE_KEY=$3" --name prodgorez btba/praha-gorez
docker run -d --name prodfiles btba/praha-nginx
docker run -d -p 80:80 -p 443:443 -P --volumes-from prodfiles --link prodgorez:gorez --name prodnginx nginx
