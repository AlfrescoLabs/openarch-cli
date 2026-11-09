## openarch

CLI for OpenArch: install and operate Hyland OpenArch

### Synopsis

openarch, the CLI for OpenArch, installs and operates Hyland OpenArch with Docker Compose.
Not an official Hyland product: an AlfrescoLabs project that Hyland does not officially support.

Every command supports --output json. Results go to stdout and logs go to stderr.
Exit codes: 0 ok, 1 error, 2 bad usage, 3 prerequisite missing,
4 service not healthy, 5 remote API error, 6 authentication failed.

```
openarch [flags]
```

### Examples

```
  openarch doctor
  openarch init --dir my-openarch
  openarch up --dir my-openarch
  openarch status --output json
  openarch logs openarch-app --tail 50
  openarch down
```

### Options

```
      --context string    connection context to use
      --dir string        workspace directory (default: current directory)
  -h, --help              help for openarch
      --no-color          disable colors (env NO_COLOR)
      --non-interactive   never prompt; fail when a value is missing
  -o, --output string     output format: text or json (env OPENARCH_OUTPUT) (default "text")
  -v, --verbose           print debug logs to stderr
```

### SEE ALSO

* [openarch api](openarch_api.md)	 - Call the OpenArch API
* [openarch backup](openarch_backup.md)	 - Back up MongoDB, the OpenArch files and the OpenSearch indices
* [openarch clean](openarch_clean.md)	 - Stop OpenArch and delete its data
* [openarch completion](openarch_completion.md)	 - Generate the autocompletion script for the specified shell
* [openarch config](openarch_config.md)	 - Manage saved connections (contexts)
* [openarch doctor](openarch_doctor.md)	 - Check the prerequisites to run OpenArch
* [openarch documents](openarch_documents.md)	 - Get, create, list, upload and download documents
* [openarch down](openarch_down.md)	 - Stop OpenArch and remove its containers, keeping the data
* [openarch init](openarch_init.md)	 - Create an OpenArch workspace
* [openarch logs](openarch_logs.md)	 - Show the logs of the services
* [openarch mcp](openarch_mcp.md)	 - Run an MCP server that exposes the commands as tools
* [openarch migrate](openarch_migrate.md)	 - Update openarch.yaml to the schema of this openarch
* [openarch plugin](openarch_plugin.md)	 - List the plugins that extend openarch
* [openarch query](openarch_query.md)	 - Run a query and list the documents found
* [openarch render](openarch_render.md)	 - Write the workspace files again from openarch.yaml
* [openarch restore](openarch_restore.md)	 - Replace the data of the workspace with a backup
* [openarch secrets](openarch_secrets.md)	 - Manage the passwords of a workspace
* [openarch self-update](openarch_self-update.md)	 - Replace this binary with the latest release
* [openarch skill](openarch_skill.md)	 - Install the Agent Skill that teaches AI coding agents to use openarch
* [openarch smoke-test](openarch_smoke-test.md)	 - Check OpenArch end to end: create, read, query and delete a document
* [openarch status](openarch_status.md)	 - Show the state of each service and of the OpenArch API
* [openarch up](openarch_up.md)	 - Start OpenArch and wait until it is healthy
* [openarch upgrade](openarch_upgrade.md)	 - Change the OpenArch version and check the result
* [openarch users](openarch_users.md)	 - Manage the OpenArch users of a workspace
* [openarch version](openarch_version.md)	 - Print version information

