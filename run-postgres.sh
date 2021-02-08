set -ex
docker rm -f postgres || true
docker run --name postgres \
    -p 127.0.0.1:5432:5432 \
    -v $(pwd)/.postgres:/var/lib/postgresql/data \
    -e POSTGRES_HOST_AUTH_METHOD=trust \
    -e PGDATA=/var/lib/postgresql/data/pgdata \
    -v "$PWD/postgres.conf":/etc/postgresql/postgresql.conf \
    --shm-size=2g \
    postgres:13.1-alpine postgres -c 'config_file=/etc/postgresql/postgresql.conf'

# --shm-size explanation: https://stackoverflow.com/a/56754077
