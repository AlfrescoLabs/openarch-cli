# OpenArch workspace: golden

This folder was created by `openarch init`. `openarch.yaml` is the source of truth.
To change the deployment, edit `openarch.yaml` and run `openarch render`.

## Endpoints

| Service | URL |
| --- | --- |
| OpenArch API | http://localhost:8080 |
| Swagger UI | http://localhost:8080/swagger-ui/index.html |
| Health | http://localhost:8080/actuator/health |

## Credentials

All passwords are in `.env.local` (mode 0600, git-ignored). Keep that file private.
Docker Compose reads it as the source of its secrets. Containers get each secret as a
file in `/run/secrets`, never as an environment variable, so `docker inspect` does not show them.

| Service | User | Password in `.env.local` |
| --- | --- | --- |
| OpenArch | `admin` | `OPENARCH_ADMIN_PASSWORD` |
| MongoDB (root) | `root` | `MONGODB_ROOT_PASSWORD` |
| MongoDB (OpenArch, database `openarch`) | `openarch` | `MONGODB_APP_PASSWORD` |
| OpenSearch (admin) | `admin` | `OPENSEARCH_ADMIN_PASSWORD` |
| OpenSearch (OpenArch) | `openarch` | `OPENSEARCH_APP_PASSWORD` |

`config/users.yml` and `config/opensearch/internal_users.yml` hold only bcrypt hashes.
After a change of the OpenArch admin password or the MongoDB OpenArch password, run
`openarch render`, then `docker compose --env-file .env --env-file .env.local up -d --force-recreate`.
The MongoDB root password and the OpenSearch passwords apply only when the data is created.
OpenSearch serves plain HTTP inside the Docker network, with users and passwords required.

## Storage

Data is kept in Docker named volumes: `golden_mongodb-data`, `golden_opensearch-data` and `golden_openarch-data`.
`docker compose down` keeps them. `docker compose down -v` deletes them.

## Run without the CLI

```
docker compose --env-file .env --env-file .env.local up -d
docker compose ps
docker compose logs -f openarch-app
docker compose down
```
