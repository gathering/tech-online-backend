#!/bin/bash

# Convenience script to dump the DB.

set -eu

docker-compose -f dev/docker-compose.yml exec db sh -c "pg_dump -U gondulapi gondulapi"