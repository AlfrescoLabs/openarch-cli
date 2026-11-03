# OpenArch workspace: golden

This folder was created by `openarch init`. `openarch.yaml` is the source of truth.
To change the deployment, edit `openarch.yaml` and run `openarch render`.

## Endpoints

| Service | URL |
| --- | --- |
| OpenArch API | https://openarch.local:8443 |
| Swagger UI | https://openarch.local:8443/swagger-ui/index.html |
| Health | https://openarch.local:8443/actuator/health |

## Credentials

The admin user and password are in `.env.local`. Keep that file private.
`config/users.yml` holds only a bcrypt hash of the password. OpenArch reads it at startup.

## TLS

The certificate in `certs/` is self-signed, so browsers and clients do not trust it by default.
It is valid for openarch.local, localhost and 127.0.0.1.

## Storage

Data is kept in Docker named volumes: `golden_mongodb-data`, `golden_opensearch-data` and `golden_openarch-data`.
`docker compose down` keeps them. `docker compose down -v` deletes them.

## Run without the CLI

```
docker compose up -d
docker compose ps
docker compose logs -f openarch-app
docker compose down
```
