# OpenArch workspace: golden

This folder was created by `openarch init`. `openarch.yaml` is the source of truth.
To change the deployment, edit `openarch.yaml` and run `openarch render`.

## Endpoints

| Service | URL |
| --- | --- |
| OpenArch API | http://localhost:8080 |
| Swagger UI | http://localhost:8080/swagger-ui/index.html |
| Health | http://localhost:8080/actuator/health |
| OpenSearch Dashboards | http://localhost:5601 |
| OpenSearch | http://localhost:9200 |
| MongoDB | mongodb://localhost:27017/?directConnection=true |

## Credentials

All passwords are in `.env.local` (mode 0600, git-ignored). Keep that file private.

| Service | User | Password in `.env.local` |
| --- | --- | --- |
| OpenArch | `admin` | `OPENARCH_ADMIN_PASSWORD` |

`security.backendAuth` is false: MongoDB and OpenSearch have no authentication.

## Storage

Data is kept in Docker named volumes: `golden_mongodb-data`, `golden_opensearch-data` and `golden_openarch-data`.
`docker compose down` keeps them. `docker compose down -v` deletes them.

## Run

```
openarch up
openarch status
openarch logs openarch-app -f
openarch down
```

## Run without the CLI

```
docker compose --env-file .env --env-file .env.local up -d
docker compose ps
docker compose logs -f openarch-app
docker compose down
```
