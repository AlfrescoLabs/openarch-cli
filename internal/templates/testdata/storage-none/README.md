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

The admin user and password are in `.env.local`. Keep that file private.
`config/users.yml` holds only a bcrypt hash of the password. OpenArch reads it at startup.

## Storage

Storage is `none`: no data is kept when the containers are removed.

## Run without the CLI

```
docker compose up -d
docker compose ps
docker compose logs -f openarch-app
docker compose down
```
